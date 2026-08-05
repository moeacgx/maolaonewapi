package common_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

type redisHashRoundTripServer struct {
	mu          sync.Mutex
	hashes      map[string]map[string]string
	generations map[string]int64
	locks       map[string]string
}

func newRedisHashRoundTripClient(server *redisHashRoundTripServer) *redis.Client {
	return redis.NewClient(&redis.Options{
		MaxRetries: -1,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			go server.serve(serverConn)
			return clientConn, nil
		},
	})
}

func readRedisCommand(reader *bufio.Reader) ([]string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	count, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(header), "*"))
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, count)
	for range count {
		lengthHeader, readErr := reader.ReadString('\n')
		if readErr != nil {
			return nil, readErr
		}
		length, parseErr := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(lengthHeader), "$"))
		if parseErr != nil || length < 0 {
			return nil, fmt.Errorf("invalid Redis bulk string length %q", lengthHeader)
		}
		payload := make([]byte, length+2)
		if _, readErr = io.ReadFull(reader, payload); readErr != nil {
			return nil, readErr
		}
		args = append(args, string(payload[:length]))
	}
	return args, nil
}

func writeRedisSimple(conn net.Conn, value string) error {
	_, err := io.WriteString(conn, "+"+value+"\r\n")
	return err
}

func writeRedisInteger(conn net.Conn, value int) error {
	_, err := fmt.Fprintf(conn, ":%d\r\n", value)
	return err
}

func (server *redisHashRoundTripServer) executeIntegerCommand(args []string) (int, error) {
	if len(args) == 0 {
		return 0, fmt.Errorf("empty Redis command")
	}
	switch strings.ToUpper(args[0]) {
	case "HSET":
		if len(args) < 4 || len(args)%2 != 0 {
			return 0, fmt.Errorf("invalid HSET arguments")
		}
		server.mu.Lock()
		defer server.mu.Unlock()
		fields := server.hashes[args[1]]
		if fields == nil {
			fields = make(map[string]string)
			server.hashes[args[1]] = fields
		}
		added := 0
		for index := 2; index < len(args); index += 2 {
			if _, exists := fields[args[index]]; !exists {
				added++
			}
			fields[args[index]] = args[index+1]
		}
		return added, nil
	case "EXPIRE", "PEXPIRE":
		return 1, nil
	case "EXISTS":
		if len(args) != 2 {
			return 0, fmt.Errorf("invalid EXISTS arguments")
		}
		server.mu.Lock()
		defer server.mu.Unlock()
		if _, exists := server.hashes[args[1]]; exists {
			return 1, nil
		}
		if _, exists := server.locks[args[1]]; exists {
			return 1, nil
		}
		if _, exists := server.generations[args[1]]; exists {
			return 1, nil
		}
		return 0, nil
	case "EVAL":
		if len(args) < 4 {
			return 0, fmt.Errorf("invalid EVAL arguments")
		}
		keyCount, err := strconv.Atoi(args[2])
		if err != nil || keyCount < 1 || len(args) < 3+keyCount {
			return 0, fmt.Errorf("invalid EVAL key count")
		}
		server.mu.Lock()
		defer server.mu.Unlock()
		if server.generations == nil {
			server.generations = make(map[string]int64)
		}
		if server.locks == nil {
			server.locks = make(map[string]string)
		}
		script := args[1]
		keys := args[3 : 3+keyCount]
		arguments := args[3+keyCount:]
		switch {
		case strings.Contains(script, "local current_owner"):
			if len(keys) != 3 || len(arguments) != 2 {
				return 0, fmt.Errorf("invalid fallback ensure script arguments")
			}
			currentOwner := server.locks[keys[2]]
			if currentOwner == arguments[0] {
				return 1, nil
			}
			if currentOwner != "" {
				return 0, nil
			}
			server.locks[keys[2]] = arguments[0]
			server.generations[keys[1]]++
			delete(server.hashes, keys[0])
			return 1, nil
		case strings.Contains(script, "local preserve_guard"):
			if len(keys) < 2 || len(keys) > 3 || len(arguments) < 5 {
				return 0, fmt.Errorf("invalid fenced hash write script arguments")
			}
			if len(keys) == 3 && server.locks[keys[2]] != "" {
				return -1, nil
			}
			currentGeneration := strconv.FormatInt(server.generations[keys[0]], 10)
			if currentGeneration != arguments[0] {
				return 0, nil
			}
			preserveCount, parseErr := strconv.Atoi(arguments[4])
			if parseErr != nil || preserveCount < 0 || len(arguments) < 5+preserveCount {
				return 0, fmt.Errorf("invalid fenced hash preserve count")
			}
			fields := server.hashes[keys[1]]
			preserved := make(map[string]string)
			canPreserve := arguments[2] == "" || fields[arguments[2]] == arguments[3]
			if canPreserve {
				for _, field := range arguments[5 : 5+preserveCount] {
					if value, exists := fields[field]; exists {
						preserved[field] = value
					}
				}
			}
			fieldValues := arguments[5+preserveCount:]
			if len(fieldValues)%2 != 0 {
				return 0, fmt.Errorf("invalid fenced hash field pairs")
			}
			fields = make(map[string]string, len(fieldValues)/2+len(preserved))
			for index := 0; index < len(fieldValues); index += 2 {
				fields[fieldValues[index]] = fieldValues[index+1]
			}
			for field, value := range preserved {
				fields[field] = value
			}
			server.hashes[keys[1]] = fields
			return 1, nil
		case strings.Contains(script, "redis.pcall(\"HINCRBY\""):
			if len(keys) != 3 || len(arguments) != 7 {
				return 0, fmt.Errorf("invalid fallback increment script arguments")
			}
			if server.locks[keys[2]] != "" {
				return -1, nil
			}
			fields := server.hashes[keys[0]]
			if fields != nil && fields[arguments[0]] == arguments[1] {
				current, parseErr := strconv.ParseInt(fields[arguments[2]], 10, 64)
				delta, deltaErr := strconv.ParseInt(arguments[3], 10, 64)
				if parseErr == nil && deltaErr == nil {
					fields[arguments[2]] = strconv.FormatInt(current+delta, 10)
					return 1, nil
				}
			}
			server.locks[keys[2]] = arguments[5]
			server.generations[keys[1]]++
			delete(server.hashes, keys[0])
			return 0, nil
		case strings.Contains(script, "redis.call(\"GET\", KEYS[3])") &&
			strings.Contains(script, "redis.call(\"DEL\", KEYS[3])"):
			if len(keys) != 3 || len(arguments) != 1 {
				return 0, fmt.Errorf("invalid fallback finish script arguments")
			}
			if server.locks[keys[2]] != arguments[0] {
				return 0, nil
			}
			server.generations[keys[1]]++
			delete(server.hashes, keys[0])
			delete(server.locks, keys[2])
			return 1, nil
		case len(keys) == 1 &&
			strings.Contains(script, "redis.call(\"GET\", KEYS[1])") &&
			strings.Contains(script, "redis.call(\"PEXPIRE\", KEYS[1], ARGV[2])"):
			if len(arguments) != 2 {
				return 0, fmt.Errorf("invalid fallback renewal script arguments")
			}
			if server.locks[keys[0]] != arguments[0] {
				return 0, nil
			}
			return 1, nil
		case strings.Contains(script, "redis.call(\"HINCRBY\", KEYS[1], ARGV[1], ARGV[2])"):
			if len(keys) != 1 || len(arguments) != 3 {
				return 0, fmt.Errorf("invalid hash increment script arguments")
			}
			fields := server.hashes[keys[0]]
			if fields == nil {
				return 0, nil
			}
			currentValue, exists := fields[arguments[0]]
			if !exists {
				return 0, nil
			}
			current, parseErr := strconv.ParseInt(currentValue, 10, 64)
			if parseErr != nil {
				return 0, parseErr
			}
			delta, deltaErr := strconv.ParseInt(arguments[1], 10, 64)
			if deltaErr != nil {
				return 0, deltaErr
			}
			fields[arguments[0]] = strconv.FormatInt(current+delta, 10)
			return 1, nil
		case strings.Contains(script, "local current_generation") &&
			strings.Contains(script, "redis.call(\"DEL\", KEYS[2])"):
			if len(keys) != 2 || len(arguments) != 1 {
				return 0, fmt.Errorf("invalid conditional generation invalidation arguments")
			}
			if strconv.FormatInt(server.generations[keys[0]], 10) != arguments[0] {
				return 0, nil
			}
			server.generations[keys[0]]++
			delete(server.hashes, keys[1])
			return 1, nil
		case strings.Contains(script, "redis.call(\"HKEYS\", KEYS[2])"):
			if len(keys) != 2 {
				return 0, fmt.Errorf("invalid generation preserve arguments")
			}
			server.generations[keys[0]]++
			fields := server.hashes[keys[1]]
			if fields == nil {
				return 0, nil
			}
			keep := make(map[string]bool, len(arguments))
			for _, field := range arguments {
				keep[field] = true
			}
			for field := range fields {
				if !keep[field] {
					delete(fields, field)
				}
			}
			return 1, nil
		case strings.Contains(script, "for index = 2, #KEYS do"):
			if len(arguments) != 0 {
				return 0, fmt.Errorf("invalid generation delete arguments")
			}
			server.generations[keys[0]]++
			for _, key := range keys[1:] {
				delete(server.hashes, key)
			}
			return len(keys) - 1, nil
		}
		return 0, fmt.Errorf("unsupported EVAL script")
	default:
		return 0, fmt.Errorf("unsupported Redis command %q", args[0])
	}
}

func (server *redisHashRoundTripServer) writeHash(conn net.Conn, key string) error {
	server.mu.Lock()
	fields := make(map[string]string, len(server.hashes[key]))
	for field, value := range server.hashes[key] {
		fields[field] = value
	}
	server.mu.Unlock()

	names := make([]string, 0, len(fields))
	for field := range fields {
		names = append(names, field)
	}
	sort.Strings(names)
	if _, err := fmt.Fprintf(conn, "*%d\r\n", len(names)*2); err != nil {
		return err
	}
	for _, field := range names {
		value := fields[field]
		if _, err := fmt.Fprintf(conn, "$%d\r\n%s\r\n$%d\r\n%s\r\n", len(field), field, len(value), value); err != nil {
			return err
		}
	}
	return nil
}

func (server *redisHashRoundTripServer) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	var queued [][]string
	for {
		args, err := readRedisCommand(reader)
		if err != nil {
			return
		}
		command := strings.ToUpper(args[0])
		switch {
		case command == "MULTI":
			queued = make([][]string, 0)
			if writeRedisSimple(conn, "OK") != nil {
				return
			}
		case command == "EXEC":
			if _, err = fmt.Fprintf(conn, "*%d\r\n", len(queued)); err != nil {
				return
			}
			for _, queuedArgs := range queued {
				value, executeErr := server.executeIntegerCommand(queuedArgs)
				if executeErr != nil {
					_, _ = fmt.Fprintf(conn, "-%s\r\n", executeErr.Error())
					continue
				}
				if writeRedisInteger(conn, value) != nil {
					return
				}
			}
			queued = nil
		case queued != nil:
			queued = append(queued, args)
			if writeRedisSimple(conn, "QUEUED") != nil {
				return
			}
		case command == "HGETALL" && len(args) == 2:
			if server.writeHash(conn, args[1]) != nil {
				return
			}
		case command == "PING":
			if writeRedisSimple(conn, "PONG") != nil {
				return
			}
		default:
			value, executeErr := server.executeIntegerCommand(args)
			if executeErr != nil {
				_, _ = fmt.Fprintf(conn, "-%s\r\n", executeErr.Error())
				continue
			}
			if writeRedisInteger(conn, value) != nil {
				return
			}
		}
	}
}

func TestRedisHashTokenCompositeFieldsRoundTrip(t *testing.T) {
	server := &redisHashRoundTripServer{hashes: make(map[string]map[string]string)}
	client := newRedisHashRoundTripClient(server)
	t.Cleanup(func() { _ = client.Close() })

	previousClient := common.RDB
	common.RDB = client
	t.Cleanup(func() { common.RDB = previousClient })

	allowedIPs := "127.0.0.1\n10.0.0.1"
	original := model.Token{
		Id:             19,
		UserId:         7,
		Status:         1,
		Name:           "round-trip-token",
		RemainQuota:    987654,
		UnlimitedQuota: false,
		AllowIps:       &allowedIPs,
		Group:          "group-a,group-b",
		GroupMode:      model.TokenGroupModeExplicit,
		GroupIds:       []int{341, 512},
		GroupDetails: []model.GroupReference{
			{Id: 341, Code: "group-a", Name: "分组 A"},
			{Id: 512, Code: "group-b", Name: "分组 B"},
		},
	}

	require.NoError(t, common.RedisHSetObj("token:round-trip", &original, time.Minute))

	var restored model.Token
	require.NoError(t, common.RedisHGetObj("token:round-trip", &restored))
	require.Equal(t, original.GroupIds, restored.GroupIds)
	require.Equal(t, original.GroupDetails, restored.GroupDetails)
	require.Equal(t, original.RemainQuota, restored.RemainQuota)
	require.Equal(t, original.AllowIps, restored.AllowIps)
	require.Equal(t, original.Name, restored.Name)
	require.Equal(t, original.GroupMode, restored.GroupMode)
}

type redisCompositeSnapshot struct {
	Ratios  map[string]float64
	Details struct {
		Enabled bool
		Name    string
	}
}

func TestRedisHashMapAndStructFieldsRoundTrip(t *testing.T) {
	server := &redisHashRoundTripServer{hashes: make(map[string]map[string]string)}
	client := newRedisHashRoundTripClient(server)
	t.Cleanup(func() { _ = client.Close() })

	previousClient := common.RDB
	common.RDB = client
	t.Cleanup(func() { common.RDB = previousClient })

	original := redisCompositeSnapshot{Ratios: map[string]float64{"default": 1, "vip": 0.2}}
	original.Details.Enabled = true
	original.Details.Name = "复合字段"

	require.NoError(t, common.RedisHSetObj("composite:round-trip", &original, time.Minute))

	var restored redisCompositeSnapshot
	require.NoError(t, common.RedisHGetObj("composite:round-trip", &restored))
	require.Equal(t, original, restored)
}

func TestRedisHGetObjDistinguishesMissingAndCorruptHashes(t *testing.T) {
	type quotaSnapshot struct {
		Quota int
	}
	server := &redisHashRoundTripServer{
		hashes: map[string]map[string]string{
			"user:corrupt":         {"Quota": "not-an-integer"},
			"user:partial":         {"Id": "42"},
			"user:profile-partial": {"Id": "42", "Quota": "90"},
			"user:profile-corrupt": {"Id": "42", "Quota": "90", "Role": "not-an-integer"},
			"user:group-corrupt":   {"Id": "42", "Quota": "90", "GroupId": "not-an-integer"},
		},
	}
	useRedisHashRoundTripServer(t, server)

	var snapshot quotaSnapshot
	err := common.RedisHGetObj("user:missing", &snapshot)
	require.ErrorIs(t, err, common.ErrRedisHashNotFound)
	require.NotErrorIs(t, err, common.ErrRedisHashCorrupt)

	err = common.RedisHGetObj("user:corrupt", &snapshot)
	require.ErrorIs(t, err, common.ErrRedisHashCorrupt)
	require.NotErrorIs(t, err, common.ErrRedisHashNotFound)
	var fieldErr *common.RedisHashFieldDecodeError
	require.True(t, errors.As(err, &fieldErr))
	require.Equal(t, "Quota", fieldErr.Field)

	err = common.RedisHGetObjWithRequiredFields("user:partial", &snapshot, "Id", "Quota")
	require.ErrorIs(t, err, common.ErrRedisHashCorrupt)
	require.ErrorIs(t, err, common.ErrRedisHashFieldMissing)
	require.True(t, errors.As(err, &fieldErr))
	require.Equal(t, "Quota", fieldErr.Field)

	var partialSnapshot struct {
		Id    int
		Quota int
		Role  int
	}
	err = common.RedisHGetObjWithRequiredFields(
		"user:profile-partial",
		&partialSnapshot,
		"Id",
		"Quota",
		"Role",
	)
	require.ErrorIs(t, err, common.ErrRedisHashFieldMissing)
	require.Equal(t, 42, partialSnapshot.Id)
	require.Equal(t, 90, partialSnapshot.Quota)

	var corruptProfileSnapshot struct {
		Id    int
		Quota int
		Role  int
	}
	err = common.RedisHGetObjWithRequiredFields(
		"user:profile-corrupt",
		&corruptProfileSnapshot,
		"Id",
		"Quota",
		"Role",
	)
	require.ErrorIs(t, err, common.ErrRedisHashCorrupt)
	require.True(t, errors.As(err, &fieldErr))
	require.Equal(t, "Role", fieldErr.Field)
	require.Equal(t, 42, corruptProfileSnapshot.Id)
	require.Equal(t, 90, corruptProfileSnapshot.Quota)

	var corruptGroupSnapshot struct {
		Id      int
		GroupId int
		Quota   int
	}
	err = common.RedisHGetObjWithRequiredFields(
		"user:group-corrupt",
		&corruptGroupSnapshot,
		"Id",
		"Quota",
		"GroupId",
	)
	require.ErrorIs(t, err, common.ErrRedisHashCorrupt)
	require.True(t, errors.As(err, &fieldErr))
	require.Equal(t, "GroupId", fieldErr.Field)
	require.Equal(t, 42, corruptGroupSnapshot.Id)
	require.Equal(t, 90, corruptGroupSnapshot.Quota)
}

func TestRedisBatchInvalidationBumpsOneGlobalGeneration(t *testing.T) {
	server := &redisHashRoundTripServer{
		hashes: map[string]map[string]string{
			"token:a": {"RemainQuota": "100"},
			"token:b": {"RemainQuota": "200"},
		},
	}
	client := newRedisHashRoundTripClient(server)
	t.Cleanup(func() { _ = client.Close() })

	previousClient := common.RDB
	common.RDB = client
	t.Cleanup(func() { common.RDB = previousClient })

	require.NoError(t, common.RedisBumpGenerationAndDeleteKeys(
		"token-cache-generation",
		[]string{"token:a", "token:b"},
	))

	server.mu.Lock()
	defer server.mu.Unlock()
	require.EqualValues(t, 1, server.generations["token-cache-generation"])
	require.NotContains(t, server.hashes, "token:a")
	require.NotContains(t, server.hashes, "token:b")
}

func useRedisHashRoundTripServer(t *testing.T, server *redisHashRoundTripServer) {
	t.Helper()
	client := newRedisHashRoundTripClient(server)
	t.Cleanup(func() { _ = client.Close() })
	previousClient := common.RDB
	common.RDB = client
	t.Cleanup(func() { common.RDB = previousClient })
}

func TestRedisHIncrByIfExistsReportsMissingAndUpdatedStates(t *testing.T) {
	server := &redisHashRoundTripServer{hashes: make(map[string]map[string]string)}
	useRedisHashRoundTripServer(t, server)

	updated, err := common.RedisHIncrByIfExists("user:501", "Quota", -5, time.Minute)
	require.NoError(t, err)
	require.False(t, updated)
	require.NotContains(t, server.hashes, "user:501")

	server.hashes["user:501"] = map[string]string{"Id": "501", "Quota": "100"}
	updated, err = common.RedisHIncrByIfExists("user:501", "Quota", -5, time.Minute)
	require.NoError(t, err)
	require.True(t, updated)
	require.Equal(t, "95", server.hashes["user:501"]["Quota"])
}

func TestRedisQuotaFallbackLockStatesAndOwnerFencedFinish(t *testing.T) {
	server := &redisHashRoundTripServer{hashes: make(map[string]map[string]string)}
	useRedisHashRoundTripServer(t, server)
	dataKey := "user:502"
	generationKey := "user-cache-generation:502"
	lockKey := "user-quota-fallback:502"

	state, err := common.RedisHIncrByOrAcquireFallback(
		dataKey, generationKey, lockKey, "Id", "502", "Quota", -5,
		time.Minute, "owner-a", time.Minute,
	)
	require.NoError(t, err)
	require.Equal(t, common.RedisHashIncrementFallbackAcquired, state)

	state, err = common.RedisHIncrByOrAcquireFallback(
		dataKey, generationKey, lockKey, "Id", "502", "Quota", -5,
		time.Minute, "owner-b", time.Minute,
	)
	require.NoError(t, err)
	require.Equal(t, common.RedisHashIncrementFallbackBusy, state)
	renewed, err := common.RedisRenewHashFallback(lockKey, "owner-b", time.Minute)
	require.NoError(t, err)
	require.False(t, renewed)
	renewed, err = common.RedisRenewHashFallback(lockKey, "owner-a", time.Minute)
	require.NoError(t, err)
	require.True(t, renewed)

	finished, err := common.RedisFinishHashFallback(dataKey, generationKey, lockKey, "owner-b")
	require.NoError(t, err)
	require.False(t, finished)
	finished, err = common.RedisFinishHashFallback(dataKey, generationKey, lockKey, "owner-a")
	require.NoError(t, err)
	require.True(t, finished)

	server.mu.Lock()
	defer server.mu.Unlock()
	require.EqualValues(t, 2, server.generations[generationKey])
	require.NotContains(t, server.locks, lockKey)
}

func TestRedisQuotaFallbackEnsureReacquiresExpiredLock(t *testing.T) {
	server := &redisHashRoundTripServer{
		hashes: map[string]map[string]string{
			"user:508": {"Id": "508", "Quota": "100"},
		},
	}
	useRedisHashRoundTripServer(t, server)

	protected, err := common.RedisEnsureHashFallback(
		"user:508",
		"user-cache-generation:508",
		"user-quota-fallback:508",
		"owner-a",
		time.Minute,
	)
	require.NoError(t, err)
	require.True(t, protected)
	require.NotContains(t, server.hashes, "user:508")
	require.EqualValues(t, 1, server.generations["user-cache-generation:508"])

	protected, err = common.RedisEnsureHashFallback(
		"user:508",
		"user-cache-generation:508",
		"user-quota-fallback:508",
		"owner-b",
		time.Minute,
	)
	require.NoError(t, err)
	require.False(t, protected)
	require.Equal(t, "owner-a", server.locks["user-quota-fallback:508"])
}

func TestRedisQuotaFallbackUpdatesCompleteHash(t *testing.T) {
	server := &redisHashRoundTripServer{
		hashes: map[string]map[string]string{
			"user:503": {"Id": "503", "Quota": "100", "Role": "1"},
		},
	}
	useRedisHashRoundTripServer(t, server)

	state, err := common.RedisHIncrByOrAcquireFallback(
		"user:503", "user-cache-generation:503", "user-quota-fallback:503",
		"Id", "503", "Quota", -5, time.Minute, "owner", time.Minute,
	)
	require.NoError(t, err)
	require.Equal(t, common.RedisHashIncrementUpdated, state)
	require.Equal(t, "95", server.hashes["user:503"]["Quota"])
	require.Empty(t, server.locks)
}

func TestRedisQuotaFallbackAcquiresForPartialOrCorruptHash(t *testing.T) {
	for name, fields := range map[string]map[string]string{
		"missing_id":    {"Quota": "100"},
		"empty_id":      {"Id": "", "Quota": "100"},
		"wrong_id":      {"Id": "999", "Quota": "100"},
		"corrupt_quota": {"Id": "504", "Quota": "not-an-integer"},
	} {
		t.Run(name, func(t *testing.T) {
			dataKey := "user:504:" + name
			generationKey := "user-cache-generation:504:" + name
			lockKey := "user-quota-fallback:504:" + name
			server := &redisHashRoundTripServer{
				hashes: map[string]map[string]string{dataKey: fields},
			}
			useRedisHashRoundTripServer(t, server)

			state, err := common.RedisHIncrByOrAcquireFallback(
				dataKey, generationKey, lockKey, "Id", "504", "Quota", -5,
				time.Minute, "owner", time.Minute,
			)
			require.NoError(t, err)
			require.Equal(t, common.RedisHashIncrementFallbackAcquired, state)
			require.NotContains(t, server.hashes, dataKey)
		})
	}
}

func TestRedisUserSnapshotWriteReportsFallbackBlock(t *testing.T) {
	type userSnapshot struct {
		Id    int
		Quota int
	}
	server := &redisHashRoundTripServer{
		hashes:      make(map[string]map[string]string),
		generations: map[string]int64{"user-cache-generation:505": 3},
		locks:       map[string]string{"user-quota-fallback:505": "owner"},
	}
	useRedisHashRoundTripServer(t, server)

	written, err := common.RedisHSetObjIfGenerationWithPreserveGuardAndBlockKey(
		"user-cache-generation:505",
		"user:505",
		3,
		&userSnapshot{Id: 505, Quota: 100},
		time.Minute,
		"Id",
		"user-quota-fallback:505",
		"Quota",
	)
	require.ErrorIs(t, err, common.ErrRedisHashWriteBlocked)
	require.False(t, written)
}

func TestRedisUserSnapshotWritePreservesQuotaOnlyAtCurrentGeneration(t *testing.T) {
	type userSnapshot struct {
		Id    int
		Quota int
		Role  int
	}
	server := &redisHashRoundTripServer{
		hashes: map[string]map[string]string{
			"user:506": {"Id": "506", "Quota": "80", "Role": "0"},
		},
		generations: map[string]int64{"user-cache-generation:506": 3},
	}
	useRedisHashRoundTripServer(t, server)

	written, err := common.RedisHSetObjIfGenerationWithPreserveGuardAndBlockKey(
		"user-cache-generation:506",
		"user:506",
		2,
		&userSnapshot{Id: 506, Quota: 100, Role: 1},
		time.Minute,
		"Id",
		"user-quota-fallback:506",
		"Quota",
	)
	require.NoError(t, err)
	require.False(t, written)
	require.Equal(t, "80", server.hashes["user:506"]["Quota"])
	require.Equal(t, "0", server.hashes["user:506"]["Role"])

	written, err = common.RedisHSetObjIfGenerationWithPreserveGuardAndBlockKey(
		"user-cache-generation:506",
		"user:506",
		3,
		&userSnapshot{Id: 506, Quota: 100, Role: 1},
		time.Minute,
		"Id",
		"user-quota-fallback:506",
		"Quota",
	)
	require.NoError(t, err)
	require.True(t, written)
	require.Equal(t, "80", server.hashes["user:506"]["Quota"])
	require.Equal(t, "1", server.hashes["user:506"]["Role"])
}

func TestRedisUserSnapshotWriteDoesNotPreserveQuotaFromWrongUser(t *testing.T) {
	type userSnapshot struct {
		Id    int
		Quota int
		Role  int
	}
	server := &redisHashRoundTripServer{
		hashes: map[string]map[string]string{
			"user:507": {"Id": "999", "Quota": "80", "Role": "0"},
		},
		generations: map[string]int64{"user-cache-generation:507": 3},
	}
	useRedisHashRoundTripServer(t, server)

	written, err := common.RedisHSetObjIfGenerationWithPreserveGuardAndBlockKey(
		"user-cache-generation:507",
		"user:507",
		3,
		&userSnapshot{Id: 507, Quota: 100, Role: 1},
		time.Minute,
		"Id",
		"user-quota-fallback:507",
		"Quota",
	)
	require.NoError(t, err)
	require.True(t, written)
	require.Equal(t, "507", server.hashes["user:507"]["Id"])
	require.Equal(t, "100", server.hashes["user:507"]["Quota"])
}
