package controller

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/extension"
	"github.com/gin-gonic/gin"
)

const extensionUploadMultipartOverheadBytes int64 = 1 << 20

type setExtensionEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

func requireExtensionUser(c *gin.Context) bool {
	if !common.IsValidateRole(c.GetInt("role")) || c.GetInt("id") <= 0 || c.GetInt("role") < common.RoleCommonUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "extension access denied"})
		return false
	}
	return true
}

func requireExtensionRoot(c *gin.Context) bool {
	if c.GetInt("role") != common.RoleRootUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "root permission required"})
		return false
	}
	return true
}

func ListExtensions(c *gin.Context) {
	if !requireExtensionUser(c) {
		return
	}
	role := c.GetInt("role")
	includeDisabled := role == common.RoleRootUser && c.Query("all") == "true"
	data := gin.H{"modules": extension.DefaultManager.List(role, includeDisabled)}
	if role == common.RoleRootUser {
		data["root"] = extension.DefaultManager.RootDir()
	}
	common.ApiSuccess(c, data)
}

func RefreshExtensions(c *gin.Context) {
	if !requireExtensionRoot(c) {
		return
	}
	if err := extension.DefaultManager.Scan(); err != nil {
		common.SysError("extension registry refresh failed")
		common.ApiErrorMsg(c, "extension registry refresh failed")
		return
	}
	common.ApiSuccess(c, gin.H{
		"root":    extension.DefaultManager.RootDir(),
		"modules": extension.DefaultManager.List(c.GetInt("role"), true),
	})
}

func UploadExtension(c *gin.Context) {
	if !requireExtensionRoot(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, extension.MaxInstallArchiveBytes+extensionUploadMultipartOverheadBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			common.ApiErrorMsg(c, "module zip file is too large")
			return
		}
		common.ApiErrorMsg(c, "module zip file is required")
		return
	}
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(fileHeader.Filename)), ".zip") {
		common.ApiErrorMsg(c, "only .zip module archives are supported")
		return
	}
	if fileHeader.Size <= 0 || fileHeader.Size > extension.MaxInstallArchiveBytes {
		common.ApiErrorMsg(c, "module zip file is too large")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		common.ApiErrorMsg(c, "module zip file cannot be opened")
		return
	}
	defer file.Close()
	readerAt, ok := file.(io.ReaderAt)
	if !ok {
		common.ApiErrorMsg(c, "module zip file cannot be read")
		return
	}
	module, err := extension.DefaultManager.InstallArchive(readerAt, fileHeader.Size)
	if err != nil {
		common.SysError("extension archive installation failed")
		common.ApiErrorMsg(c, "module archive could not be installed")
		return
	}
	common.ApiSuccess(c, gin.H{
		"root":   extension.DefaultManager.RootDir(),
		"module": module.Public(true),
	})
}

func SetExtensionEnabled(c *gin.Context) {
	if !requireExtensionRoot(c) {
		return
	}
	var req setExtensionEnabledRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid extension state request")
		return
	}
	module, err := extension.DefaultManager.SetEnabled(c.Param("id"), req.Enabled)
	if err != nil {
		common.ApiErrorMsg(c, "extension state could not be changed")
		return
	}
	common.ApiSuccess(c, module.Public(true))
}

func UninstallExtension(c *gin.Context) {
	if !requireExtensionRoot(c) {
		return
	}
	if err := extension.DefaultManager.Uninstall(c.Param("id")); err != nil {
		common.ApiErrorMsg(c, "extension could not be uninstalled")
		return
	}
	common.ApiSuccess(c, gin.H{
		"root":    extension.DefaultManager.RootDir(),
		"modules": extension.DefaultManager.List(c.GetInt("role"), true),
	})
}

func ProxyExtension(c *gin.Context) {
	if !requireExtensionUser(c) {
		return
	}
	proxy, err := extension.DefaultManager.ProxyHandler(
		c.Param("id"),
		c.Param("path"),
		c.GetInt("role"),
		extension.ProxyContext{
			UserID:         strconv.Itoa(c.GetInt("id")),
			Username:       c.GetString("username"),
			Role:           strconv.Itoa(c.GetInt("role")),
			Group:          c.GetString("group"),
			UseAccessToken: strconv.FormatBool(c.GetBool("use_access_token")),
		},
	)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "extension proxy unavailable"})
		return
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func GetExtensionNativeAsset(c *gin.Context) {
	if !requireExtensionUser(c) {
		return
	}
	asset, err := extension.DefaultManager.OpenNativeAsset(
		c.Param("id"),
		c.Param("pageKey"),
		c.Param("target"),
		c.Param("asset"),
		c.GetInt("role"),
	)
	if err != nil {
		status := http.StatusForbidden
		message := "native extension asset unavailable"
		if errors.Is(err, extension.ErrNativeAssetNotFound) {
			status = http.StatusNotFound
			message = "native extension asset not found"
		}
		c.JSON(status, gin.H{"success": false, "message": message})
		return
	}
	defer asset.File.Close()

	c.Header("Content-Type", asset.ContentType)
	c.Header("Content-Length", strconv.FormatInt(asset.Size, 10))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, asset.File); err != nil {
		_ = c.Error(err)
	}
}

func GetExtensionHostContext(c *gin.Context) {
	if !requireExtensionUser(c) {
		return
	}
	common.ApiSuccess(c, gin.H{
		"user_id":          c.GetInt("id"),
		"username":         c.GetString("username"),
		"role":             c.GetInt("role"),
		"group":            c.GetString("group"),
		"use_access_token": c.GetBool("use_access_token"),
		"version":          common.Version,
	})
}
