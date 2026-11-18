package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"runtime"
	"strings"
	"syscall"

	"hestia/internal/index"
)

func (s *Server) handleDeleteAccessCode(w http.ResponseWriter, r *http.Request) {
	if !localLibraryAccess(r) {
		errJSON(w, http.StatusForbidden, "请在 Hestia 所在电脑上查看授权码")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"code": s.deleteAccessCode})
}

func (s *Server) canManageMedia(r *http.Request) bool {
	return localLibraryAccess(r) || s.deleteAccessCode != "" &&
		subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Hestia-Delete-Code")), []byte(s.deleteAccessCode)) == 1
}

// A browser can only manage files from the app's local address on this machine.
func localLibraryAccess(r *http.Request) bool {
	host := r.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.Trim(host, "[]")
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	peer, address := net.ParseIP(remote), net.ParseIP(host)
	if peer.IsLoopback() {
		return strings.EqualFold(host, "localhost") || address.IsLoopback()
	}
	// A specific LAN bind can also receive requests from the same machine.
	// Match the literal host to both socket endpoints, never forwarded headers.
	local, ok := r.Context().Value(http.LocalAddrContextKey).(*net.TCPAddr)
	return ok && peer != nil && address != nil && peer.Equal(local.IP) && address.Equal(local.IP)
}

func (s *Server) handleFolderDeletePreview(w http.ResponseWriter, r *http.Request) {
	if !s.canManageMedia(r) {
		errJSON(w, http.StatusForbidden, "请输入电脑端的删除授权码")
		return
	}
	preview, err := s.store.PreviewFolderDeletion(r.PathValue("id"))
	if err != nil {
		s.folderDeleteError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleFolderDelete(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if !s.canManageMedia(r) || r.Header.Get("Origin") != scheme+"://"+r.Host {
		errJSON(w, http.StatusForbidden, "删除授权无效，请重新打开删除窗口")
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		errJSON(w, http.StatusUnsupportedMediaType, "请求格式错误")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var body struct {
		Reason string `json:"reason"`
		Token  string `json:"token"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&body); err != nil || len(body.Token) != 64 {
		errJSON(w, http.StatusBadRequest, "请先确认删除范围")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		errJSON(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	result, err := s.store.DeleteFolderMedia(r.PathValue("id"), body.Reason, body.Token, func(folderPath string, folderIDs []string) (func(), error) {
		return s.prepareFolderDeletion(r.Context(), folderPath, folderIDs)
	})
	if err != nil {
		s.folderDeleteError(w, err)
		return
	}
	s.prog.UpdateMediaIDs(result.Removed, result.Moved)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) folderDeleteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, index.ErrFolderUnavailable):
		errJSON(w, http.StatusNotFound, err.Error())
	case errors.Is(err, index.ErrDeleteReason):
		errJSON(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, index.ErrDeleteConflict), errors.Is(err, errMediaInUse), errors.Is(err, errFolderDeleting):
		errJSON(w, http.StatusConflict, err.Error())
	case runtime.GOOS == "windows" && (errors.Is(err, windowsSharingViolation) || errors.Is(err, windowsLockViolation)):
		s.logger.Printf("删除媒体文件时遇到占用: %v", err)
		errJSON(w, http.StatusConflict, "文件仍被其他程序占用。关闭正在读取该目录的程序后重试。"+err.Error())
	default:
		s.logger.Printf("删除媒体文件操作失败: %v", err)
		errJSON(w, http.StatusConflict, "无法处理该文件夹，请检查文件是否被占用及目录权限。"+err.Error())
	}
}

const (
	windowsSharingViolation = syscall.Errno(32)
	windowsLockViolation    = syscall.Errno(33)
)
