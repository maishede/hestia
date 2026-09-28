package server

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"hestia/internal/index"
)

// Toolchain 记录 ffmpeg / ffprobe 的探测结果。
type Toolchain struct {
	FFmpeg  string `json:"ffmpegPath"`
	FFprobe string `json:"ffprobePath"`
}

func DetectToolchain() Toolchain {
	var tc Toolchain
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	exe := ".exe"
	if runtime.GOOS != "windows" {
		exe = ""
	}
	find := func(name string) string {
		for _, dir := range []string{exeDir, filepath.Join(exeDir, "bin"), filepath.Join(exeDir, "ffmpeg", "bin")} {
			p := filepath.Join(dir, name+exe)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p
			}
		}
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
		return ""
	}
	tc.FFmpeg = find("ffmpeg")
	tc.FFprobe = find("ffprobe")
	return tc
}

func (t Toolchain) FFmpegOK() bool   { return t.FFmpeg != "" }
func (t Toolchain) FFprobeOK() bool { return t.FFprobe != "" }

type ProbeResult struct {
	Duration float64
	VCodec   string
	ACodec   string
	Width    int
	Height   int
	Subs     []index.SubStream
}

func RunFFprobe(ffprobe, path string) (*ProbeResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffprobe,
		"-v", "error", "-print_format", "json",
		"-show_entries", "format=duration:stream=index,codec_type,codec_name,width,height:stream_tags=language,title",
		path)
	hideChildWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var probe struct {
		Streams []struct {
			Index     int    `json:"index"`
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			Tags      struct {
				Language string `json:"language"`
				Title    string `json:"title"`
			} `json:"stream_tags"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, err
	}
	res := &ProbeResult{}
	res.Duration, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	for _, st := range probe.Streams {
		if st.CodecType == "video" && res.VCodec == "" {
			res.VCodec, res.Width, res.Height = st.CodecName, st.Width, st.Height
		}
		if st.CodecType == "audio" && res.ACodec == "" {
			res.ACodec = st.CodecName
		}
		if st.CodecType == "subtitle" {
			res.Subs = append(res.Subs, index.SubStream{
				Index: st.Index, Codec: st.CodecName,
				Title: st.Tags.Title, Lang: st.Tags.Language,
			})
		}
	}
	return res, nil
}

// DecidePlayback 依据探测编码 + 容器 + 客户端 UA 判断是否建议转码。
// Safari（含 iOS）可硬解 HEVC/AC3，允许直连；其余浏览器遇到则直接建议转码。
// 未探测过（Probed=false）返回 direct，由前端 video.error 事件兜底切换。
func DecidePlayback(m index.Media, ua string) (need bool, reason string) {
	switch strings.ToLower(m.Ext) {
	case ".rm", ".rmvb":
		return true, "容器格式不受浏览器支持"
	}
	if !m.Probed {
		return false, ""
	}
	isSafari := strings.Contains(ua, "Safari") && !strings.Contains(ua, "Chrome") && !strings.Contains(ua, "Edg/")
	if !isSafari {
		switch m.VCodec {
		case "hevc", "h265":
			return true, "HEVC 视频编码不受支持"
		}
		switch m.ACodec {
		case "ac3", "eac3", "dts", "truehd":
			return true, "音频编码不受支持（" + m.ACodec + "）"
		}
	}
	return false, ""
}
