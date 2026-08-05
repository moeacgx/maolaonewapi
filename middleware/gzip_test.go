package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func TestDecompressRequestMiddlewareZstd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := []byte(`{"model":"gpt-test","input":"hello"}`)
	encoder, err := zstd.NewWriter(nil)
	require.NoError(t, err)
	compressed := encoder.EncodeAll(payload, nil)
	encoder.Close()

	var received []byte
	var contentEncoding string
	router := gin.New()
	router.Use(DecompressRequestMiddleware())
	router.POST("/", func(c *gin.Context) {
		contentEncoding = c.GetHeader("Content-Encoding")
		received, err = io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(compressed))
	req.Header.Set("Content-Encoding", "zstd")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Equal(t, payload, received)
	require.Empty(t, contentEncoding)
}

func TestDecompressRequestMiddlewareRejectsInvalidZstd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := false
	var readErr error
	router := gin.New()
	router.Use(DecompressRequestMiddleware())
	router.POST("/", func(c *gin.Context) {
		called = true
		_, readErr = io.ReadAll(c.Request.Body)
		if readErr != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("not-zstd"))
	req.Header.Set("Content-Encoding", "zstd")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.True(t, called)
	require.Error(t, readErr)
}
