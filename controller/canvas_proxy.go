package controller

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

const (
	canvasMultipartMaxParts       = 128
	canvasMultipartMaxHeaderBytes = 16 << 10
	canvasMultipartMaxNameBytes   = 128
	canvasMultipartMaxFilename    = 255
)

func CanvasPrepareRequest(c *gin.Context) {
	group := strings.TrimSpace(c.Query("group"))
	if group == "" {
		abortCanvasRequest(c, http.StatusBadRequest, "group is required")
		return
	}
	identity, authenticated := middleware.GetAuthIdentity(c)
	if !authenticated || identity.UserID != c.GetInt("id") || identity.SessionID == "" || identity.UserAuthVersion <= 0 || identity.SessionVersion <= 0 || !middleware.CanvasRequestOriginTrusted(c.Request) {
		abortCanvasRequest(c, http.StatusForbidden, "trusted canvas session is required")
		return
	}

	userID := c.GetInt("id")
	user, err := model.GetUserCache(userID)
	if err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to load user")
		return
	}
	if identity.UserAuthVersion != user.AuthVersion {
		abortCanvasRequest(c, http.StatusForbidden, "trusted canvas session is required")
		return
	}
	usable := service.GetUserUsableGroups(user.Group)
	_, permitted := usable[group]
	if group != "auto" {
		permitted = permitted && ratio_setting.ContainsGroupRatio(group)
	}
	if !permitted {
		abortCanvasRequest(c, http.StatusForbidden, "selected group is not available")
		return
	}

	user.WriteContext(c)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
	temporaryToken := &model.Token{UserId: userID, Name: "canvas-" + group, Group: group}
	if err := middleware.SetupContextForToken(c, temporaryToken); err != nil {
		abortCanvasRequest(c, http.StatusInternalServerError, "failed to prepare canvas session")
		return
	}
	// Establish this capability only after the live session, auth version,
	// exact Canvas origin, selected group, and synthetic token context have all
	// been validated. Client input cannot create it.
	common.SetContextKey(c, constant.ContextKeyTokenQuotaExempt, true)
	common.SetContextKey(c, constant.ContextKeyCanvasTrusted, true)

	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		if err := injectCanvasGroup(c, group); err != nil {
			status := http.StatusBadRequest
			if common.IsRequestBodyTooLargeError(err) {
				status = http.StatusRequestEntityTooLarge
			}
			abortCanvasRequest(c, status, "invalid canvas request body")
			return
		}
	}
	c.Next()
}

func CanvasListModels(c *gin.Context) {
	ListModels(c, constant.ChannelTypeOpenAI)
}

func CanvasChatCompletions(c *gin.Context)  { Relay(c, types.RelayFormatOpenAI) }
func CanvasImageGenerations(c *gin.Context) { Relay(c, types.RelayFormatOpenAIImage) }
func CanvasImageEdits(c *gin.Context)       { Relay(c, types.RelayFormatOpenAIImage) }
func CanvasAudioSpeech(c *gin.Context)      { Relay(c, types.RelayFormatOpenAIAudio) }
func CanvasVideoSubmit(c *gin.Context)      { RelayTask(c) }
func CanvasVideoFetch(c *gin.Context)       { RelayTaskFetch(c) }
func CanvasVideoContent(c *gin.Context)     { VideoProxy(c) }

func abortCanvasRequest(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": types.OpenAIError{
		Message: message,
		Type:    "invalid_request_error",
	}})
}

func injectCanvasGroup(c *gin.Context, group string) error {
	contentType := strings.TrimSpace(c.GetHeader("Content-Type"))
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil && contentType != "" {
		return errors.New("invalid content type")
	}
	switch mediaType {
	case "", gin.MIMEJSON:
		return injectCanvasJSONGroup(c, group)
	case gin.MIMEMultipartPOSTForm:
		return injectCanvasMultipartGroup(c, group, contentType)
	default:
		return nil
	}
}

func injectCanvasJSONGroup(c *gin.Context, group string) error {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}
	reader, err := storage.NewReader()
	if err != nil {
		return err
	}
	defer reader.Close()

	payload := make(map[string]any)
	if storage.Size() > 0 {
		if err := common.DecodeJson(reader, &payload); err != nil {
			return err
		}
	}
	payload["group"] = group
	body, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	if int64(len(body)) > canvasRequestBodyLimit() {
		return common.ErrRequestBodyTooLarge
	}
	next, err := common.CreateBodyStorage(body)
	if err != nil {
		return err
	}
	_ = reader.Close()
	installCanvasBodyStorage(c, next, gin.MIMEJSON)
	return nil
}

func injectCanvasMultipartGroup(c *gin.Context, group, contentType string) error {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil || strings.TrimSpace(params["boundary"]) == "" {
		return errors.New("invalid multipart boundary")
	}
	source, err := common.GetBodyStorage(c)
	if err != nil {
		return err
	}
	sourceReader, err := source.NewReader()
	if err != nil {
		return err
	}
	defer sourceReader.Close()

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	nextContentType := writer.FormDataContentType()
	go func() {
		copyErr := copyCanvasMultipart(writer, multipart.NewReader(sourceReader, params["boundary"]), group)
		_ = pipeWriter.CloseWithError(copyErr)
	}()

	estimatedSize := source.Size() + 1024 + int64(len(group))
	next, createErr := common.CreateBodyStorageFromReader(pipeReader, estimatedSize, canvasRequestBodyLimit())
	_ = pipeReader.Close()
	_ = sourceReader.Close()
	if createErr != nil {
		return createErr
	}
	installCanvasBodyStorage(c, next, nextContentType)
	return nil
}

func copyCanvasMultipart(writer *multipart.Writer, reader *multipart.Reader, group string) error {
	if err := writer.WriteField("group", group); err != nil {
		return err
	}
	parts := 0
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return writer.Close()
		}
		if err != nil {
			return err
		}
		parts++
		if parts > canvasMultipartMaxParts {
			_ = part.Close()
			return errors.New("too many multipart parts")
		}
		if err := validateCanvasMultipartPart(part); err != nil {
			_ = part.Close()
			return err
		}
		if part.FormName() == "group" {
			_ = part.Close()
			continue
		}
		out, err := createCanvasMultipartPart(writer, part)
		if err == nil {
			err = copyCanvasMultipartContent(out, part)
		}
		_ = part.Close()
		if err != nil {
			return err
		}
	}
}

func copyCanvasMultipartContent(destination io.Writer, source *multipart.Part) error {
	if source.FileName() == "" {
		_, err := io.Copy(destination, source)
		return err
	}
	probe := make([]byte, 512)
	n, err := io.ReadFull(source, probe)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	probe = probe[:n]
	if len(probe) == 0 {
		return errors.New("multipart file is empty")
	}
	declared, _, _ := mime.ParseMediaType(source.Header.Get("Content-Type"))
	detected := http.DetectContentType(probe)
	if detected != "application/octet-stream" && !strings.EqualFold(detected, declared) {
		return errors.New("multipart file type does not match content")
	}
	if _, err := destination.Write(probe); err != nil {
		return err
	}
	_, err = io.Copy(destination, source)
	return err
}

func validateCanvasMultipartPart(part *multipart.Part) error {
	dispositionType, disposition, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	if err != nil || !strings.EqualFold(dispositionType, "form-data") {
		return errors.New("invalid multipart content disposition")
	}
	name := disposition["name"]
	filename, hasFilename := disposition["filename"]
	if name != part.FormName() || !validCanvasMultipartToken(name, canvasMultipartMaxNameBytes) {
		return errors.New("invalid multipart field name")
	}
	if hasFilename {
		if !validCanvasMultipartToken(filename, canvasMultipartMaxFilename) || strings.ContainsAny(filename, `/\\`) || filename == "." || filename == ".." {
			return errors.New("invalid multipart filename")
		}
	}
	headerBytes := 0
	for key, values := range part.Header {
		if !validCanvasMultipartToken(key, canvasMultipartMaxNameBytes) {
			return errors.New("invalid multipart header")
		}
		for _, value := range values {
			headerBytes += len(key) + len(value)
			if strings.ContainsAny(value, "\r\n\x00") {
				return errors.New("invalid multipart header value")
			}
		}
	}
	if headerBytes > canvasMultipartMaxHeaderBytes {
		return errors.New("multipart headers are too large")
	}
	if hasFilename {
		contentType := strings.TrimSpace(part.Header.Get("Content-Type"))
		if contentType == "" {
			return errors.New("multipart file content type is required")
		}
		mediaType, params, err := mime.ParseMediaType(contentType)
		if err != nil || len(params) != 0 || !allowedCanvasUploadType(mediaType) {
			return errors.New("unsupported multipart file type")
		}
	}
	return nil
}

func createCanvasMultipartPart(writer *multipart.Writer, source *multipart.Part) (io.Writer, error) {
	filename := source.FileName()
	if filename == "" {
		return writer.CreateFormField(source.FormName())
	}
	header := make(textproto.MIMEHeader, 2)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name": source.FormName(), "filename": filename,
	}))
	header.Set("Content-Type", strings.ToLower(strings.TrimSpace(source.Header.Get("Content-Type"))))
	return writer.CreatePart(header)
}

func validCanvasMultipartToken(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func allowedCanvasUploadType(contentType string) bool {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png", "image/jpeg", "image/webp", "image/gif", "image/avif":
		return true
	default:
		return false
	}
}

func installCanvasBodyStorage(c *gin.Context, storage common.BodyStorage, contentType string) {
	common.CleanupBodyStorage(c)
	c.Set(common.KeyBodyStorage, storage)
	c.Set(common.KeyRequestBody, nil)
	c.Request.Body = io.NopCloser(common.NewReplayableBodyReader(storage))
	c.Request.ContentLength = storage.Size()
	c.Request.Header.Set("Content-Length", fmt.Sprint(storage.Size()))
	c.Request.Header.Set("Content-Type", contentType)
}

func canvasRequestBodyLimit() int64 {
	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	return int64(maxMB) << 20
}

// readCanvasMultipartModel scans only small text metadata and never buffers an
// uploaded file. The caller owns reader.
func readCanvasMultipartModel(reader io.Reader, boundary string) string {
	multipartReader := multipart.NewReader(bufio.NewReader(reader), boundary)
	for {
		part, err := multipartReader.NextPart()
		if err != nil {
			return ""
		}
		if part.FormName() != "model" || part.FileName() != "" {
			_ = part.Close()
			continue
		}
		value, err := io.ReadAll(io.LimitReader(part, 4097))
		_ = part.Close()
		if err != nil || len(value) > 4096 {
			return ""
		}
		return strings.TrimSpace(string(bytes.TrimSpace(value)))
	}
}
