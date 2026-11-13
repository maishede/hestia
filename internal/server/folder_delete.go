package server

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"

	"hestia/internal/index"
)

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
	if !localLibraryAccess(r) {
		errJSON(w, http.StatusForbidden, "请在 Hestia 所在电脑上使用本机地址")
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
	if !localLibraryAccess(r) || r.Header.Get("Origin") != scheme+"://"+r.Host {
		errJSON(w, http.StatusForbidden, "请在 Hestia 所在电脑上使用本机地址")
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
	result, err := s.store.DeleteFolderMedia(r.PathValue("id"), body.Reason, body.Token)
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
	case errors.Is(err, index.ErrDeleteConflict):
		errJSON(w, http.StatusConflict, err.Error())
	default:
		s.logger.Printf("删除媒体文件操作失败: %v", err)
		errJSON(w, http.StatusConflict, "无法处理该文件夹，请检查文件是否被占用及目录权限。"+err.Error())
	}
}
