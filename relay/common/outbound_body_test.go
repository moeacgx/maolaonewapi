package common

import (
	"io"
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestNewOutboundJSONBodyReplayReadersAreIndependent(t *testing.T) {
	payload := []byte(`{"model":"test-model","input":"abcdefghijklmnopqrstuvwxyz"}`)

	body, closer, err := NewOutboundJSONBody(payload)
	require.NoError(t, err)
	defer closer.Close()
	require.EqualValues(t, len(payload), body.Size())

	primaryPrefix := make([]byte, 8)
	_, err = io.ReadFull(body, primaryPrefix)
	require.NoError(t, err)
	require.Equal(t, payload[:8], primaryPrefix)

	replayA, err := body.NewReader()
	require.NoError(t, err)
	defer replayA.Close()
	replayB, err := body.NewReader()
	require.NoError(t, err)
	defer replayB.Close()

	aPrefix := make([]byte, 5)
	_, err = io.ReadFull(replayA, aPrefix)
	require.NoError(t, err)
	require.Equal(t, payload[:5], aPrefix)

	bAll, err := io.ReadAll(replayB)
	require.NoError(t, err)
	require.Equal(t, payload, bAll)

	aRest, err := io.ReadAll(replayA)
	require.NoError(t, err)
	require.Equal(t, payload[5:], aRest)

	primaryRest, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, payload[8:], primaryRest)

	require.NoError(t, closer.Close())
	_, err = body.NewReader()
	require.ErrorIs(t, err, rootcommon.ErrStorageClosed)
}
