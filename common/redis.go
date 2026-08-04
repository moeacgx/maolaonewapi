package common

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

var RDB *redis.Client
var RedisEnabled = true

func RedisKeyCacheSeconds() int {
	return SyncFrequency
}

// InitRedisClient This function is called after init()
func InitRedisClient() (err error) {
	if os.Getenv("REDIS_CONN_STRING") == "" {
		RedisEnabled = false
		SysLog("REDIS_CONN_STRING not set, Redis is not enabled")
		return nil
	}
	if os.Getenv("SYNC_FREQUENCY") == "" {
		SysLog("SYNC_FREQUENCY not set, use default value 60")
		SyncFrequency = 60
	}
	SysLog("Redis is enabled")
	opt, err := redis.ParseURL(os.Getenv("REDIS_CONN_STRING"))
	if err != nil {
		FatalLog("failed to parse Redis connection string: " + err.Error())
	}
	opt.PoolSize = GetEnvOrDefault("REDIS_POOL_SIZE", 10)
	RDB = redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = RDB.Ping(ctx).Result()
	if err != nil {
		FatalLog("Redis ping test failed: " + err.Error())
	}
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis connected to %s", opt.Addr))
		SysLog(fmt.Sprintf("Redis database: %d", opt.DB))
	}
	return err
}

func ParseRedisOption() *redis.Options {
	opt, err := redis.ParseURL(os.Getenv("REDIS_CONN_STRING"))
	if err != nil {
		FatalLog("failed to parse Redis connection string: " + err.Error())
	}
	return opt
}

func RedisSet(key string, value string, expiration time.Duration) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis SET: key=%s, value=%s, expiration=%v", key, value, expiration))
	}
	ctx := context.Background()
	return RDB.Set(ctx, key, value, expiration).Err()
}

func RedisGet(key string) (string, error) {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis GET: key=%s", key))
	}
	ctx := context.Background()
	val, err := RDB.Get(ctx, key).Result()
	return val, err
}

//func RedisExpire(key string, expiration time.Duration) error {
//	ctx := context.Background()
//	return RDB.Expire(ctx, key, expiration).Err()
//}
//
//func RedisGetEx(key string, expiration time.Duration) (string, error) {
//	ctx := context.Background()
//	return RDB.GetSet(ctx, key, expiration).Result()
//}

func RedisDel(key string) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis DEL: key=%s", key))
	}
	ctx := context.Background()
	return RDB.Del(ctx, key).Err()
}

func RedisDelKey(key string) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis DEL Key: key=%s", key))
	}
	ctx := context.Background()
	return RDB.Del(ctx, key).Err()
}

func RedisDelKeys(keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if RDB == nil {
		return errors.New("redis client is nil")
	}
	ctx := context.Background()
	return RDB.Del(ctx, keys...).Err()
}

func redisHashObjectData(obj interface{}) (map[string]interface{}, error) {
	data := make(map[string]interface{})
	// 使用反射遍历结构体字段
	valueOfObj := reflect.ValueOf(obj)
	if valueOfObj.Kind() != reflect.Ptr || valueOfObj.IsNil() {
		return nil, fmt.Errorf("obj must be a non-nil pointer to a struct, got %T", obj)
	}
	v := valueOfObj.Elem()
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("obj must be a pointer to a struct, got pointer to %T", v.Interface())
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		// Skip DeletedAt field
		if field.Type.String() == "gorm.DeletedAt" {
			continue
		}

		// 处理指针类型
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				data[field.Name] = ""
				continue
			}
			value = value.Elem()
		}

		switch value.Kind() {
		case reflect.Bool:
			data[field.Name] = strconv.FormatBool(value.Bool())
		case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct, reflect.Interface:
			encoded, err := Marshal(value.Interface())
			if err != nil {
				return nil, fmt.Errorf("failed to marshal Redis hash field %s: %w", field.Name, err)
			}
			data[field.Name] = string(encoded)
		default:
			// 标量保持原有字符串编码，兼容已有缓存。
			data[field.Name] = fmt.Sprintf("%v", value.Interface())
		}
	}
	return data, nil
}

func RedisHSetObj(key string, obj interface{}, expiration time.Duration) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis HSET: key=%s, obj=%+v, expiration=%v", key, obj, expiration))
	}
	ctx := context.Background()
	data, err := redisHashObjectData(obj)
	if err != nil {
		return err
	}

	txn := RDB.TxPipeline()
	txn.HSet(ctx, key, data)

	// 只有在 expiration 大于 0 时才设置过期时间
	if expiration > 0 {
		txn.Expire(ctx, key, expiration)
	}

	_, err = txn.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to execute transaction: %w", err)
	}
	return nil
}

const redisHSetObjIfGenerationScript = `
if #KEYS >= 3 and redis.call("EXISTS", KEYS[3]) == 1 then
    return -1
end

local current_generation = redis.call("GET", KEYS[1])
if current_generation == false then
    current_generation = "0"
end
if current_generation ~= ARGV[1] then
    return 0
end

local preserve_guard = ARGV[3]
local preserve_guard_value = ARGV[4]
local preserve_count = tonumber(ARGV[5])
local preserved_fields = {}
local preserved_values = {}
local can_preserve = preserve_guard == ""
    or redis.call("HGET", KEYS[2], preserve_guard) == preserve_guard_value
if redis.call("EXISTS", KEYS[2]) == 1 and can_preserve then
    for index = 1, preserve_count do
        local field = ARGV[5 + index]
        local value = redis.call("HGET", KEYS[2], field)
        if value ~= false then
            table.insert(preserved_fields, field)
            table.insert(preserved_values, value)
        end
    end
end

local field_start = 6 + preserve_count
for index = field_start, #ARGV, 2 do
    redis.call("HSET", KEYS[2], ARGV[index], ARGV[index + 1])
end
for index = 1, #preserved_fields do
    redis.call("HSET", KEYS[2], preserved_fields[index], preserved_values[index])
end

local expiration_ms = tonumber(ARGV[2])
if expiration_ms > 0 then
    redis.call("PEXPIRE", KEYS[2], expiration_ms)
end
return 1
`

func RedisGetGeneration(key string) (int64, error) {
	if RDB == nil {
		return 0, errors.New("redis client is nil")
	}
	generation, err := RDB.Get(context.Background(), key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return generation, err
}

func RedisKeyExists(key string) (bool, error) {
	if RDB == nil {
		return false, errors.New("redis client is nil")
	}
	count, err := RDB.Exists(context.Background(), key).Result()
	if err != nil {
		return false, err
	}
	return count == 1, nil
}

var (
	ErrRedisHashWriteBlocked = errors.New("redis hash write is blocked")
	ErrRedisHashNotFound     = errors.New("redis hash does not exist")
	ErrRedisHashCorrupt      = errors.New("redis hash field is corrupt")
	ErrRedisHashFieldMissing = errors.New("required redis hash field is missing")
)

// RedisHashFieldDecodeError 标识缓存 Hash 中无法解析的具体字段。
type RedisHashFieldDecodeError struct {
	Field string
	Err   error
}

func (err *RedisHashFieldDecodeError) Error() string {
	return fmt.Sprintf("failed to decode Redis hash field %s: %v", err.Field, err.Err)
}

func (err *RedisHashFieldDecodeError) Unwrap() error {
	return err.Err
}

func (err *RedisHashFieldDecodeError) Is(target error) bool {
	return target == ErrRedisHashCorrupt
}

// RedisHSetObjIfGeneration 仅在 generation 未变时回填 hash，并可保留实时字段。
func RedisHSetObjIfGeneration(
	generationKey string,
	dataKey string,
	expectedGeneration int64,
	obj interface{},
	expiration time.Duration,
	preserveFields ...string,
) (bool, error) {
	return redisHSetObjIfGeneration(
		generationKey,
		dataKey,
		expectedGeneration,
		obj,
		expiration,
		"",
		"",
		preserveFields...,
	)
}

// RedisHSetObjIfGenerationWithPreserveGuard 仅在保护字段与对象值一致时保留实时字段。
func RedisHSetObjIfGenerationWithPreserveGuard(
	generationKey string,
	dataKey string,
	expectedGeneration int64,
	obj interface{},
	expiration time.Duration,
	preserveGuard string,
	preserveFields ...string,
) (bool, error) {
	return redisHSetObjIfGeneration(
		generationKey,
		dataKey,
		expectedGeneration,
		obj,
		expiration,
		preserveGuard,
		"",
		preserveFields...,
	)
}

// RedisHSetObjIfGenerationWithPreserveGuardAndBlockKey 在阻塞键不存在时回填快照。
func RedisHSetObjIfGenerationWithPreserveGuardAndBlockKey(
	generationKey string,
	dataKey string,
	expectedGeneration int64,
	obj interface{},
	expiration time.Duration,
	preserveGuard string,
	blockKey string,
	preserveFields ...string,
) (bool, error) {
	return redisHSetObjIfGeneration(
		generationKey,
		dataKey,
		expectedGeneration,
		obj,
		expiration,
		preserveGuard,
		blockKey,
		preserveFields...,
	)
}

func redisHSetObjIfGeneration(
	generationKey string,
	dataKey string,
	expectedGeneration int64,
	obj interface{},
	expiration time.Duration,
	preserveGuard string,
	blockKey string,
	preserveFields ...string,
) (bool, error) {
	if DebugEnabled {
		SysLog(fmt.Sprintf(
			"Redis fenced HSET: generation_key=%s, data_key=%s, generation=%d, obj=%+v, expiration=%v, guard=%s, fields=%v",
			generationKey,
			dataKey,
			expectedGeneration,
			obj,
			expiration,
			preserveGuard,
			preserveFields,
		))
	}
	if RDB == nil {
		return false, errors.New("redis client is nil")
	}
	data, err := redisHashObjectData(obj)
	if err != nil {
		return false, err
	}
	preserveGuardValue := ""
	if preserveGuard != "" {
		guardValue, exists := data[preserveGuard]
		if !exists {
			return false, fmt.Errorf("preserve guard field %s is missing from Redis hash object", preserveGuard)
		}
		preserveGuardValue = fmt.Sprint(guardValue)
	}
	arguments := make([]interface{}, 0, 5+len(preserveFields)+len(data)*2)
	arguments = append(
		arguments,
		expectedGeneration,
		expiration.Milliseconds(),
		preserveGuard,
		preserveGuardValue,
		len(preserveFields),
	)
	for _, field := range preserveFields {
		arguments = append(arguments, field)
	}
	for field, value := range data {
		arguments = append(arguments, field, value)
	}
	keys := []string{generationKey, dataKey}
	if blockKey != "" {
		keys = append(keys, blockKey)
	}
	written, err := RDB.Eval(
		context.Background(),
		redisHSetObjIfGenerationScript,
		keys,
		arguments...,
	).Int64()
	if err != nil {
		return false, fmt.Errorf("failed to atomically set Redis hash: %w", err)
	}
	if written == -1 {
		return false, ErrRedisHashWriteBlocked
	}
	return written == 1, nil
}

const redisBumpGenerationAndDeleteKeysScript = `
redis.call("INCR", KEYS[1])
for index = 2, #KEYS do
    redis.call("DEL", KEYS[index])
end
return #KEYS - 1
`

const redisBumpGenerationAndKeepHashFieldsScript = `
redis.call("INCR", KEYS[1])

if redis.call("EXISTS", KEYS[2]) == 0 then
    return 0
end

local keep = {}
for index = 1, #ARGV do
    keep[ARGV[index]] = true
end

local fields = redis.call("HKEYS", KEYS[2])
for _, field in ipairs(fields) do
    if keep[field] ~= true then
        redis.call("HDEL", KEYS[2], field)
    end
end
return 1
`

// RedisBumpGenerationAndKeepHashFields 使旧快照失效，同时保留 hash 中的实时字段。
func RedisBumpGenerationAndKeepHashFields(generationKey string, dataKey string, keepFields ...string) error {
	if RDB == nil {
		return errors.New("redis client is nil")
	}
	arguments := make([]interface{}, 0, len(keepFields))
	for _, field := range keepFields {
		arguments = append(arguments, field)
	}
	if _, err := RDB.Eval(
		context.Background(),
		redisBumpGenerationAndKeepHashFieldsScript,
		[]string{generationKey, dataKey},
		arguments...,
	).Result(); err != nil {
		return fmt.Errorf("failed to invalidate Redis hash while preserving fields: %w", err)
	}
	return nil
}

func RedisBumpGenerationAndDeleteKeys(generationKey string, dataKeys []string) error {
	if len(dataKeys) == 0 {
		return nil
	}
	if generationKey == "" {
		return errors.New("redis generation key is empty")
	}
	if RDB == nil {
		return errors.New("redis client is nil")
	}
	keys := make([]string, 1, len(dataKeys)+1)
	keys[0] = generationKey
	keys = append(keys, dataKeys...)
	return RDB.Eval(context.Background(), redisBumpGenerationAndDeleteKeysScript, keys).Err()
}

const redisBumpGenerationAndDeleteIfCurrentScript = `
local current_generation = redis.call("GET", KEYS[1])
if current_generation == false then
    current_generation = "0"
end
if current_generation ~= ARGV[1] then
    return 0
end
redis.call("INCR", KEYS[1])
redis.call("DEL", KEYS[2])
return 1
`

func RedisBumpGenerationAndDeleteIfCurrent(
	generationKey string,
	dataKey string,
	expectedGeneration int64,
) (bool, error) {
	if RDB == nil {
		return false, errors.New("redis client is nil")
	}
	deleted, err := RDB.Eval(
		context.Background(),
		redisBumpGenerationAndDeleteIfCurrentScript,
		[]string{generationKey, dataKey},
		expectedGeneration,
	).Int64()
	if err != nil {
		return false, err
	}
	return deleted == 1, nil
}

func RedisHGetObj(key string, obj interface{}) error {
	return redisHGetObj(key, obj, nil)
}

// RedisHGetObjWithRequiredFields 读取 Hash，并把缺失的必需字段视为损坏缓存。
func RedisHGetObjWithRequiredFields(key string, obj interface{}, requiredFields ...string) error {
	return redisHGetObj(key, obj, requiredFields)
}

func decodeRedisHashField(fieldValue reflect.Value, fieldName string, value string) error {
	// 先解引用指针字段，空字符串沿用旧行为并保持 nil。
	if fieldValue.Kind() == reflect.Ptr {
		if value == "" {
			return nil
		}
		if fieldValue.IsNil() {
			fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
		}
		fieldValue = fieldValue.Elem()
	}

	// 按缓存结构字段的实际类型解码。
	switch fieldValue.Kind() {
	case reflect.String:
		fieldValue.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intValue, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return &RedisHashFieldDecodeError{Field: fieldName, Err: err}
		}
		fieldValue.SetInt(intValue)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uintValue, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return &RedisHashFieldDecodeError{Field: fieldName, Err: err}
		}
		fieldValue.SetUint(uintValue)
	case reflect.Float32, reflect.Float64:
		floatValue, err := strconv.ParseFloat(value, fieldValue.Type().Bits())
		if err != nil {
			return &RedisHashFieldDecodeError{Field: fieldName, Err: err}
		}
		fieldValue.SetFloat(floatValue)
	case reflect.Bool:
		boolValue, err := strconv.ParseBool(value)
		if err != nil {
			return &RedisHashFieldDecodeError{Field: fieldName, Err: err}
		}
		fieldValue.SetBool(boolValue)
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct, reflect.Interface:
		if err := UnmarshalJsonStr(value, fieldValue.Addr().Interface()); err != nil {
			return &RedisHashFieldDecodeError{Field: fieldName, Err: err}
		}
	default:
		return fmt.Errorf("unsupported field type: %s for field %s", fieldValue.Kind(), fieldName)
	}
	return nil
}

func redisHGetObj(key string, obj interface{}, requiredFields []string) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis HGETALL: key=%s", key))
	}
	ctx := context.Background()
	if RDB == nil {
		return errors.New("redis client is nil")
	}

	result, err := RDB.HGetAll(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to load hash from Redis: %w", err)
	}

	if len(result) == 0 {
		return fmt.Errorf("%w: key %s", ErrRedisHashNotFound, key)
	}
	// Handle both pointer and non-pointer values
	val := reflect.ValueOf(obj)
	if val.Kind() != reflect.Ptr {
		return fmt.Errorf("obj must be a pointer to a struct, got %T", obj)
	}

	v := val.Elem()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("obj must be a pointer to a struct, got pointer to %T", v.Interface())
	}

	t := v.Type()
	fieldIndexes := make(map[string]int, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		fieldIndexes[t.Field(i).Name] = i
	}
	decodedFields := make(map[string]struct{}, len(requiredFields))
	decodeField := func(fieldName string, required bool) error {
		if _, decoded := decodedFields[fieldName]; decoded {
			return nil
		}
		value, exists := result[fieldName]
		if !exists {
			if required {
				return &RedisHashFieldDecodeError{Field: fieldName, Err: ErrRedisHashFieldMissing}
			}
			return nil
		}
		decodedFields[fieldName] = struct{}{}
		fieldIndex, isStructField := fieldIndexes[fieldName]
		if !isStructField {
			return nil
		}
		return decodeRedisHashField(v.Field(fieldIndex), fieldName, value)
	}

	// 必需字段按调用方给定顺序先解码。用户缓存把 Id、Quota 放在最前，
	// 后续资料字段损坏时仍能明确判断实时额度是否可信。
	for _, fieldName := range requiredFields {
		if err := decodeField(fieldName, true); err != nil {
			return err
		}
	}
	for i := 0; i < v.NumField(); i++ {
		if err := decodeField(t.Field(i).Name, false); err != nil {
			return err
		}
	}

	return nil
}

// RedisIncr Add this function to handle atomic increments
func RedisIncr(key string, delta int64) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis INCR: key=%s, delta=%d", key, delta))
	}
	// 检查键的剩余生存时间
	ttlCmd := RDB.TTL(context.Background(), key)
	ttl, err := ttlCmd.Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return fmt.Errorf("failed to get TTL: %w", err)
	}

	// 只有在 key 存在且有 TTL 时才需要特殊处理
	if ttl > 0 {
		ctx := context.Background()
		// 开始一个Redis事务
		txn := RDB.TxPipeline()

		// 减少余额
		decrCmd := txn.IncrBy(ctx, key, delta)
		if err := decrCmd.Err(); err != nil {
			return err // 如果减少失败，则直接返回错误
		}

		// 重新设置过期时间，使用原来的过期时间
		txn.Expire(ctx, key, ttl)

		// 执行事务
		_, err = txn.Exec(ctx)
		return err
	}
	return nil
}

var ErrRedisHashFieldNotFound = errors.New("redis hash or field does not exist")

func RedisHIncrBy(key, field string, delta int64) error {
	updated, err := RedisHIncrByIfExists(key, field, delta, 0)
	if err != nil {
		return err
	}
	if !updated {
		return ErrRedisHashFieldNotFound
	}
	return nil
}

// RedisHIncrByIfExists 仅在 hash 与目标字段均存在时执行增量，并明确返回是否更新。
func RedisHIncrByIfExists(
	key string,
	field string,
	delta int64,
	expiration time.Duration,
) (bool, error) {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis HINCRBY: key=%s, field=%s, delta=%d", key, field, delta))
	}
	if RDB == nil {
		return false, errors.New("redis client is nil")
	}
	updated, err := RDB.Eval(
		context.Background(),
		redisHashIncrementIfExistsScript,
		[]string{key},
		field,
		delta,
		expiration.Milliseconds(),
	).Int64()
	if err != nil {
		return false, fmt.Errorf("failed to atomically increment Redis hash field: %w", err)
	}
	return updated == 1, nil
}

const redisHashIncrementIfExistsScript = `
if redis.call("EXISTS", KEYS[1]) == 0 or redis.call("HEXISTS", KEYS[1], ARGV[1]) == 0 then
    return 0
end
redis.call("HINCRBY", KEYS[1], ARGV[1], ARGV[2])
local expiration_ms = tonumber(ARGV[3])
if expiration_ms > 0 then
    redis.call("PEXPIRE", KEYS[1], expiration_ms)
end
return 1
`

type RedisHashIncrementState int64

const (
	RedisHashIncrementFallbackBusy     RedisHashIncrementState = -1
	RedisHashIncrementFallbackAcquired RedisHashIncrementState = 0
	RedisHashIncrementUpdated          RedisHashIncrementState = 1
)

const redisHIncrByOrAcquireFallbackScript = `
if redis.call("EXISTS", KEYS[3]) == 1 then
    return -1
end

if redis.call("EXISTS", KEYS[1]) == 1
    and redis.call("HEXISTS", KEYS[1], ARGV[1]) == 1
    and redis.call("HGET", KEYS[1], ARGV[1]) == ARGV[2]
    and redis.call("HEXISTS", KEYS[1], ARGV[3]) == 1 then
    local increment_result = redis.pcall("HINCRBY", KEYS[1], ARGV[3], ARGV[4])
    if type(increment_result) == "number" then
        local expiration_ms = tonumber(ARGV[5])
        if expiration_ms > 0 then
            redis.call("PEXPIRE", KEYS[1], expiration_ms)
        end
        return 1
    end
end

local lock_set = redis.call("SET", KEYS[3], ARGV[6], "NX", "PX", ARGV[7])
if lock_set == false then
    return -1
end
redis.call("INCR", KEYS[2])
redis.call("DEL", KEYS[1])
return 0
`

// RedisHIncrByOrAcquireFallback 原子地更新完整 hash；缓存缺失时取得跨实例回退锁。
func RedisHIncrByOrAcquireFallback(
	dataKey string,
	generationKey string,
	lockKey string,
	requiredField string,
	requiredValue string,
	field string,
	delta int64,
	expiration time.Duration,
	lockToken string,
	lockExpiration time.Duration,
) (RedisHashIncrementState, error) {
	if RDB == nil {
		return RedisHashIncrementFallbackBusy, errors.New("redis client is nil")
	}
	if lockToken == "" {
		return RedisHashIncrementFallbackBusy, errors.New("redis fallback lock token is empty")
	}
	if lockExpiration <= 0 {
		return RedisHashIncrementFallbackBusy, errors.New("redis fallback lock expiration must be positive")
	}
	state, err := RDB.Eval(
		context.Background(),
		redisHIncrByOrAcquireFallbackScript,
		[]string{dataKey, generationKey, lockKey},
		requiredField,
		requiredValue,
		field,
		delta,
		expiration.Milliseconds(),
		lockToken,
		lockExpiration.Milliseconds(),
	).Int64()
	if err != nil {
		return RedisHashIncrementFallbackBusy, fmt.Errorf("failed to update Redis hash or acquire fallback lock: %w", err)
	}
	switch RedisHashIncrementState(state) {
	case RedisHashIncrementUpdated,
		RedisHashIncrementFallbackAcquired,
		RedisHashIncrementFallbackBusy:
		return RedisHashIncrementState(state), nil
	default:
		return RedisHashIncrementFallbackBusy, fmt.Errorf("unexpected Redis hash increment state: %d", state)
	}
}

const redisFinishHashFallbackScript = `
if redis.call("GET", KEYS[3]) ~= ARGV[1] then
    return 0
end
redis.call("INCR", KEYS[2])
redis.call("DEL", KEYS[1])
redis.call("DEL", KEYS[3])
return 1
`

// RedisFinishHashFallback 仅由锁持有者结束回退窗口并清除期间产生的缓存快照。
func RedisFinishHashFallback(
	dataKey string,
	generationKey string,
	lockKey string,
	lockToken string,
) (bool, error) {
	if RDB == nil {
		return false, errors.New("redis client is nil")
	}
	finished, err := RDB.Eval(
		context.Background(),
		redisFinishHashFallbackScript,
		[]string{dataKey, generationKey, lockKey},
		lockToken,
	).Int64()
	if err != nil {
		return false, fmt.Errorf("failed to finish Redis hash fallback: %w", err)
	}
	return finished == 1, nil
}

const redisRenewHashFallbackScript = `
if redis.call("GET", KEYS[1]) ~= ARGV[1] then
    return 0
end
redis.call("PEXPIRE", KEYS[1], ARGV[2])
return 1
`

// RedisRenewHashFallback 仅允许当前持有者延长回退锁，避免数据库故障期间保护窗口过期。
func RedisRenewHashFallback(lockKey string, lockToken string, expiration time.Duration) (bool, error) {
	if RDB == nil {
		return false, errors.New("redis client is nil")
	}
	if lockToken == "" {
		return false, errors.New("redis fallback lock token is empty")
	}
	if expiration <= 0 {
		return false, errors.New("redis fallback lock expiration must be positive")
	}
	renewed, err := RDB.Eval(
		context.Background(),
		redisRenewHashFallbackScript,
		[]string{lockKey},
		lockToken,
		expiration.Milliseconds(),
	).Int64()
	if err != nil {
		return false, fmt.Errorf("failed to renew Redis hash fallback: %w", err)
	}
	return renewed == 1, nil
}

const redisEnsureHashFallbackScript = `
local current_owner = redis.call("GET", KEYS[3])
if current_owner == ARGV[1] then
    redis.call("PEXPIRE", KEYS[3], ARGV[2])
    return 1
end
if current_owner ~= false then
    return 0
end
local lock_set = redis.call("SET", KEYS[3], ARGV[1], "NX", "PX", ARGV[2])
if lock_set == false then
    return 0
end
redis.call("INCR", KEYS[2])
redis.call("DEL", KEYS[1])
return 1
`

// RedisEnsureHashFallback 续期当前锁；锁已过期且无人持有时，以同一令牌重建保护窗口。
func RedisEnsureHashFallback(
	dataKey string,
	generationKey string,
	lockKey string,
	lockToken string,
	expiration time.Duration,
) (bool, error) {
	if RDB == nil {
		return false, errors.New("redis client is nil")
	}
	if lockToken == "" {
		return false, errors.New("redis fallback lock token is empty")
	}
	if expiration <= 0 {
		return false, errors.New("redis fallback lock expiration must be positive")
	}
	protected, err := RDB.Eval(
		context.Background(),
		redisEnsureHashFallbackScript,
		[]string{dataKey, generationKey, lockKey},
		lockToken,
		expiration.Milliseconds(),
	).Int64()
	if err != nil {
		return false, fmt.Errorf("failed to ensure Redis hash fallback: %w", err)
	}
	return protected == 1, nil
}

const redisHashFieldUpdateIfExistsScript = `
if redis.call("EXISTS", KEYS[1]) == 0 then
    return 0
end
redis.call(ARGV[1], KEYS[1], ARGV[2], ARGV[3])
return 1
`

func RedisHSetField(key, field string, value interface{}) error {
	if DebugEnabled {
		SysLog(fmt.Sprintf("Redis HSET field: key=%s, field=%s, value=%v", key, field, value))
	}
	if RDB == nil {
		return errors.New("redis client is nil")
	}
	return RDB.Eval(
		context.Background(),
		redisHashFieldUpdateIfExistsScript,
		[]string{key},
		"HSET",
		field,
		value,
	).Err()
}
