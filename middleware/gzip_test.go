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

func TestDecompressRequestMiddlewareSupportsZstd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	payload := []byte(`{"model":"test","messages":[{"role":"user","content":"hello"}]}`)
	var compressed bytes.Buffer
	encoder, err := zstd.NewWriter(&compressed)
	require.NoError(t, err)
	_, err = encoder.Write(payload)
	require.NoError(t, err)
	require.NoError(t, encoder.Close())

	router := gin.New()
	router.Use(DecompressRequestMiddleware())
	router.POST("/test", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Empty(t, c.GetHeader("Content-Encoding"))
		require.Equal(t, payload, body)
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(compressed.Bytes()))
	request.Header.Set("Content-Encoding", "zstd")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestDecompressRequestMiddlewareRejectsInvalidZstd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(DecompressRequestMiddleware())
	router.POST("/test", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		require.Error(t, err)
		c.AbortWithStatus(http.StatusBadRequest)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte("not-zstd")))
	request.Header.Set("Content-Encoding", "zstd")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}
