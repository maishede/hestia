// Generate small local fixtures with FFmpeg: node fixtures.cjs <output-directory>
const { mkdirSync, writeFileSync } = require('node:fs')
const { resolve, join } = require('node:path')
const { spawnSync } = require('node:child_process')
const dir = resolve(process.argv[2] || 'player-test-media')
const films = join(dir, 'Films')
mkdirSync(films, { recursive: true })
const cues = (prefix) => `1\n00:00:01,000 --> 00:00:10,000\nEARLY_${prefix}_SUBTITLE\n\n2\n00:00:18,000 --> 00:00:28,000\nLATE_${prefix}_SUBTITLE\n`
writeFileSync(join(films, 'direct.srt'), cues('EXTERNAL'))
writeFileSync(join(films, 'transcode.srt'), cues('EXTERNAL'))
writeFileSync(join(dir, 'embedded.srt'), cues('EMBEDDED'))
writeFileSync(join(films, 'direct.ass'), `[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H80000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:10.00,Default,,0,0,0,,SECOND_TRACK\n`)
function ffmpeg(args) {
  const result = spawnSync(process.env.FFMPEG || 'ffmpeg', ['-hide_banner', '-loglevel', 'error', '-y', ...args], { stdio: 'inherit' })
  if (result.error || result.status !== 0) throw result.error || new Error(`ffmpeg exited ${result.status}`)
}
ffmpeg(['-f', 'lavfi', '-i', 'testsrc2=size=320x180:rate=24', '-f', 'lavfi', '-i', 'sine=frequency=440:sample_rate=48000', '-t', '45', '-c:v', 'libx264', '-preset', 'ultrafast', '-c:a', 'aac', '-shortest', join(films, 'direct.mp4')])
ffmpeg(['-i', join(films, 'direct.mp4'), '-i', join(dir, 'embedded.srt'), '-map', '0:v:0', '-map', '0:a:0', '-map', '1:0', '-c:v', 'libx265', '-x265-params', 'pools=2:log-level=error', '-preset', 'ultrafast', '-c:a', 'ac3', '-c:s', 'srt', join(films, 'transcode.mkv')])
ffmpeg(['-f', 'lavfi', '-i', 'testsrc=size=321x181:rate=24', '-t', '9', '-c:v', 'libx265', '-pix_fmt', 'yuv444p10le', '-x265-params', 'pools=2:log-level=error', '-preset', 'ultrafast', '-an', join(films, 'transcode-10bit.mkv')])
console.log(`Fixture library: ${dir}`)
