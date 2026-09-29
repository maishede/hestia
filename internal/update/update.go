// Package update checks the project's GitHub releases and stages verified binaries.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	latestURL    = "https://api.github.com/repos/maishede/hestia/releases/latest"
	latestPage   = "https://github.com/maishede/hestia/releases/latest"
	assetPrefix  = "/maishede/hestia/releases/download/"
	maxRelease   = 2 << 20
	maxBinary    = 150 << 20
	maxChecksums = 1 << 20
)

type Release struct {
	Version     string `json:"version"`
	Tag         string `json:"tag"`
	URL         string `json:"url"`
	Available   bool   `json:"available"`
	AssetURL    string `json:"-"`
	ChecksumURL string `json:"-"`
	Digest      string `json:"-"`
	Size        int64  `json:"-"`
}

type Client struct {
	http        *http.Client
	latestURL   string
	assetHost   string
	assetPrefix string
}

func NewClient() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 20 * time.Second
	return &Client{
		http:        &http.Client{Transport: transport},
		latestURL:   latestURL,
		assetHost:   "github.com",
		assetPrefix: assetPrefix,
	}
}

func CompareVersions(a, b string) (int, error) {
	parse := func(v string) ([3]uint64, error) {
		var result [3]uint64
		parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
		if len(parts) != 3 {
			return result, fmt.Errorf("无效版本号 %q", v)
		}
		for i, part := range parts {
			if part == "" || strings.Trim(part, "0123456789") != "" {
				return result, fmt.Errorf("无效版本号 %q", v)
			}
			n, err := strconv.ParseUint(part, 10, 64)
			if err != nil {
				return result, fmt.Errorf("无效版本号 %q: %w", v, err)
			}
			result[i] = n
		}
		return result, nil
	}
	av, err := parse(a)
	if err != nil {
		return 0, err
	}
	bv, err := parse(b)
	if err != nil {
		return 0, err
	}
	for i := range av {
		if av[i] < bv[i] {
			return -1, nil
		}
		if av[i] > bv[i] {
			return 1, nil
		}
	}
	return 0, nil
}

func (c *Client) Check(ctx context.Context, current string) (Release, error) {
	var payload struct {
		Tag    string `json:"tag_name"`
		URL    string `json:"html_url"`
		Assets []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Digest string `json:"digest"`
			Size   int64  `json:"size"`
		} `json:"assets"`
	}
	if err := c.getJSON(ctx, c.latestURL, &payload); err != nil {
		return c.checkViaRedirect(ctx, current)
	}
	comparison, err := CompareVersions(payload.Tag, current)
	if err != nil {
		return Release{}, err
	}
	r := Release{Version: strings.TrimPrefix(payload.Tag, "v"), Tag: payload.Tag, URL: payload.URL, Available: comparison > 0}
	if !r.Available {
		return r, nil
	}
	for _, a := range payload.Assets {
		switch a.Name {
		case "Hestia.exe":
			r.AssetURL, r.Digest, r.Size = a.URL, a.Digest, a.Size
		case "SHA256SUMS.txt":
			r.ChecksumURL = a.URL
		}
	}
	if !c.validAssetURL(r.AssetURL, r.Tag) {
		return Release{}, errors.New("新版 Release 缺少可信的 Hestia.exe 下载项")
	}
	if r.Digest != "" {
		if _, err := parseDigest(r.Digest); err != nil {
			return Release{}, err
		}
	} else if !c.validAssetURL(r.ChecksumURL, r.Tag) {
		return Release{}, errors.New("新版 Release 缺少 SHA-256 校验值")
	}
	if r.Size < 1 || r.Size > maxBinary {
		return Release{}, errors.New("新版程序大小异常")
	}
	return r, nil
}

// GitHub's unauthenticated REST API has a shared per-IP limit. The public
// /releases/latest redirect supplies the stable tag when that limit is hit.
func (c *Client) checkViaRedirect(ctx context.Context, current string) (Release, error) {
	request, err := c.newRequest(ctx, latestPage)
	if err != nil {
		return Release{}, err
	}
	request.Method = http.MethodHead
	response, err := c.http.Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("无法检查 GitHub 最新版本: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub 最新版本页面返回 HTTP %d", response.StatusCode)
	}
	if response.Request == nil || response.Request.URL == nil {
		return Release{}, errors.New("GitHub 未返回有效的最新版本地址")
	}
	u := response.Request.URL
	const tagPrefix = "/maishede/hestia/releases/tag/"
	if u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || !strings.HasPrefix(u.EscapedPath(), tagPrefix) {
		return Release{}, errors.New("GitHub 未返回有效的最新版本地址")
	}
	tag, err := url.PathUnescape(strings.TrimPrefix(u.EscapedPath(), tagPrefix))
	if err != nil || strings.Contains(tag, "/") {
		return Release{}, errors.New("GitHub 最新版本标签无效")
	}
	comparison, err := CompareVersions(tag, current)
	if err != nil {
		return Release{}, err
	}
	r := Release{Version: strings.TrimPrefix(tag, "v"), Tag: tag, URL: u.String(), Available: comparison > 0}
	if r.Available {
		base := "https://github.com" + assetPrefix + url.PathEscape(tag) + "/"
		r.AssetURL = base + "Hestia.exe"
		r.ChecksumURL = base + "SHA256SUMS.txt"
	}
	return r, nil
}

func (c *Client) Download(ctx context.Context, release Release, destination string, progress func(int64, int64)) error {
	if !release.Available || !c.validAssetURL(release.AssetURL, release.Tag) || release.Size < 0 || release.Size > maxBinary {
		return errors.New("更新信息无效")
	}
	expected := release.Digest
	if expected == "" {
		if !c.validAssetURL(release.ChecksumURL, release.Tag) {
			return errors.New("缺少可信的校验文件")
		}
		body, err := c.getBytes(ctx, release.ChecksumURL, maxChecksums)
		if err != nil {
			return err
		}
		expected, err = checksumFor(body, "Hestia.exe")
		if err != nil {
			return err
		}
	}
	expectedHash, err := parseDigest(expected)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	part := destination + ".part"
	f, err := os.Create(part)
	if err != nil {
		return err
	}
	defer os.Remove(part)
	defer f.Close()
	request, err := c.newRequest(ctx, release.AssetURL)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载更新失败：HTTP %d", response.StatusCode)
	}
	expectedSize := release.Size
	if expectedSize == 0 && response.ContentLength > 0 {
		expectedSize = response.ContentLength
	}
	hash := sha256.New()
	counting := &progressWriter{total: expectedSize, callback: progress}
	n, err := io.Copy(io.MultiWriter(f, hash, counting), io.LimitReader(response.Body, maxBinary+1))
	if err != nil {
		return err
	}
	if n == 0 || n > maxBinary || (expectedSize > 0 && n != expectedSize) {
		return errors.New("下载文件大小与 Release 信息不一致")
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expectedHash) {
		return errors.New("下载文件 SHA-256 校验失败")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var signature [2]byte
	if _, err := io.ReadFull(f, signature[:]); err != nil || signature != [2]byte{'M', 'Z'} {
		return errors.New("下载文件不是 Windows 可执行程序")
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(part, destination)
}

func (c *Client) getJSON(ctx context.Context, address string, dst any) error {
	body, err := c.getBytes(ctx, address, maxRelease)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("Release 信息格式错误: %w", err)
	}
	return nil
}

func (c *Client) getBytes(ctx context.Context, address string, limit int64) ([]byte, error) {
	request, err := c.newRequest(ctx, address)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 请求失败：HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("GitHub 响应过大")
	}
	return data, nil
}

func (c *Client) newRequest(ctx context.Context, address string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "Hestia-Updater")
	return request, nil
}

func (c *Client) validAssetURL(raw, tag string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), c.assetHost) || u.Port() != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return strings.HasPrefix(u.EscapedPath(), c.assetPrefix+url.PathEscape(tag)+"/")
}

func parseDigest(raw string) (string, error) {
	raw = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(raw)), "sha256:")
	if len(raw) != 64 {
		return "", errors.New("无效的 SHA-256 校验值")
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return "", errors.New("无效的 SHA-256 校验值")
	}
	return raw, nil
}

func checksumFor(data []byte, name string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return fields[0], nil
		}
	}
	return "", errors.New("校验文件中没有 Hestia.exe")
}

type progressWriter struct {
	done     int64
	total    int64
	callback func(int64, int64)
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.done += int64(len(p))
	if w.callback != nil {
		w.callback(w.done, w.total)
	}
	return len(p), nil
}
