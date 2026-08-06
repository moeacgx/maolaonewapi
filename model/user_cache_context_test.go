package model

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

type cancelAfterHGetAllHook struct {
	target   int32
	seen     atomic.Int32
	canceled atomic.Bool
	cancel   context.CancelFunc
}

func (hook *cancelAfterHGetAllHook) BeforeProcess(ctx context.Context, _ redis.Cmder) (context.Context, error) {
	return ctx, nil
}

func (hook *cancelAfterHGetAllHook) AfterProcess(_ context.Context, cmd redis.Cmder) error {
	if strings.EqualFold(cmd.Name(), "hgetall") && hook.seen.Add(1) == hook.target {
		hook.canceled.Store(true)
		hook.cancel()
	}
	return nil
}

func (hook *cancelAfterHGetAllHook) BeforeProcessPipeline(
	ctx context.Context,
	_ []redis.Cmder,
) (context.Context, error) {
	return ctx, nil
}

func (hook *cancelAfterHGetAllHook) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

type userCacheContextRedisServer struct {
	hgetallResponses [][]string
	hgetallCalls     atomic.Int32
	evalCalls        atomic.Int32
	evalAfterCancel  atomic.Int32
	canceled         *atomic.Bool
}

func readUserCacheContextRedisCommand(reader *bufio.Reader) ([]string, error) {
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

func writeUserCacheContextRedisArray(conn net.Conn, values []string) error {
	if _, err := fmt.Fprintf(conn, "*%d\r\n", len(values)); err != nil {
		return err
	}
	for _, value := range values {
		if _, err := fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(value), value); err != nil {
			return err
		}
	}
	return nil
}

func (server *userCacheContextRedisServer) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		args, err := readUserCacheContextRedisCommand(reader)
		if err != nil || len(args) == 0 {
			return
		}
		switch strings.ToUpper(args[0]) {
		case "HGETALL":
			index := int(server.hgetallCalls.Add(1)) - 1
			var response []string
			if index < len(server.hgetallResponses) {
				response = server.hgetallResponses[index]
			}
			if writeUserCacheContextRedisArray(conn, response) != nil {
				return
			}
		case "GET":
			if _, err = io.WriteString(conn, "$1\r\n0\r\n"); err != nil {
				return
			}
		case "EVAL":
			server.evalCalls.Add(1)
			if server.canceled.Load() {
				server.evalAfterCancel.Add(1)
			}
			if _, err = io.WriteString(conn, ":1\r\n"); err != nil {
				return
			}
		default:
			if _, err = io.WriteString(conn, "$-1\r\n"); err != nil {
				return
			}
		}
	}
}

func useUserCacheContextRedis(
	t *testing.T,
	cancelOnHGetAll int32,
	hgetallResponses ...[]string,
) (context.Context, *cancelAfterHGetAllHook, *userCacheContextRedisServer) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	hook := &cancelAfterHGetAllHook{target: cancelOnHGetAll, cancel: cancel}
	server := &userCacheContextRedisServer{
		hgetallResponses: hgetallResponses,
		canceled:         &hook.canceled,
	}
	client := redis.NewClient(&redis.Options{
		MaxRetries: -1,
		PoolSize:   1,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			clientConn, serverConn := net.Pipe()
			go server.serve(serverConn)
			return clientConn, nil
		},
	})
	client.AddHook(hook)

	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		cancel()
		_ = client.Close()
		common.RDB = previousRDB
		common.RedisEnabled = previousRedisEnabled
	})
	return ctx, hook, server
}

func completeUserCacheContextFields(id int, role string) []string {
	return []string{
		"Id", strconv.Itoa(id),
		"Quota", "100",
		"Group", "default",
		"GroupId", "1",
		"Email", "user@example.com",
		"Status", strconv.Itoa(common.UserStatusEnabled),
		"Role", role,
		"Username", "context-user",
		"Setting", "{}",
	}
}

func TestGetUserCacheWithRetryInvalidationRespectsContext(t *testing.T) {
	tests := []struct {
		name   string
		userId int
		fields []string
	}{
		{
			name:   "mismatched user",
			userId: 908,
			fields: completeUserCacheContextFields(999, strconv.Itoa(common.RoleCommonUser)),
		},
		{
			name:   "invalid role",
			userId: 909,
			fields: completeUserCacheContextFields(909, "2"),
		},
		{
			name:   "partial hash preserving quota",
			userId: 910,
			fields: []string{"Id", "910", "Quota", "100"},
		},
		{
			name:   "corrupt identity",
			userId: 911,
			fields: []string{"Id", "invalid", "Quota", "100"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isolateUserQuotaBatchStore(t)
			ctx, hook, server := useUserCacheContextRedis(t, 1, test.fields)

			_, err := getUserCacheWithRetry(ctx, test.userId, 0, nil)

			require.True(t, hook.canceled.Load())
			require.ErrorIs(t, err, context.Canceled)
			require.EqualValues(t, 1, server.hgetallCalls.Load())
			require.Zero(t, server.evalCalls.Load())
		})
	}
}

func TestGetUserCacheWithRetryRefreshInvalidationRespectsContext(t *testing.T) {
	isolateUserQuotaBatchStore(t)
	userId := 91000806
	user := &User{
		Id:       userId,
		Username: "refresh-context-user",
		Group:    "default",
		GroupId:  1,
		Email:    "refresh-context@example.com",
		Quota:    100,
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
		Setting:  "{}",
	}
	require.NoError(t, DB.Create(user).Error)
	t.Cleanup(func() {
		_ = DB.Unscoped().Delete(&User{}, userId).Error
	})

	corruptRefresh := completeUserCacheContextFields(userId, "invalid")
	ctx, hook, server := useUserCacheContextRedis(t, 2, nil, corruptRefresh)

	_, err := getUserCacheWithRetry(ctx, userId, 0, nil)

	require.True(t, hook.canceled.Load())
	require.ErrorIs(t, err, context.Canceled)
	require.EqualValues(t, 2, server.hgetallCalls.Load())
	require.EqualValues(t, 1, server.evalCalls.Load())
	require.Zero(t, server.evalAfterCancel.Load())
}
