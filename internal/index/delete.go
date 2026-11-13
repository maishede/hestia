package index

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	ErrFolderUnavailable = errors.New("文件夹不存在或已移动")
	ErrDeleteConflict    = errors.New("无法按当前确认内容删除")
	ErrDeleteReason      = errors.New("请输入有效的删除原因")
)

type DeletePreview struct {
	Name       string `json:"name"`
	Files      int    `json:"files"`
	Subfolders int    `json:"subfolders"`
	Token      string `json:"token"`
}

type FolderDeletion struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Files     int               `json:"files"`
	Warning   string            `json:"warning,omitempty"`
	Removed   []string          `json:"-"`
	Moved     map[string]string `json:"-"`
	libraryID string
}

type folderTarget struct {
	folder Folder
	lib    LibraryState
	parent *os.Root
	dir    *os.Root
	info   fs.FileInfo
	name   string
}

func (s *Store) openFolderTarget(id string) (folderTarget, error) {
	f, ok := s.GetFolder(id)
	if !ok {
		return folderTarget{}, ErrFolderUnavailable
	}
	lib, ok := s.GetLibrary(f.LibraryID)
	rel := filepath.FromSlash(f.RelPath)
	if !ok || !lib.Enabled || rel == "." || !filepath.IsLocal(rel) {
		return folderTarget{}, ErrFolderUnavailable
	}
	root, err := os.OpenRoot(lib.Path)
	if err != nil {
		return folderTarget{}, ErrFolderUnavailable
	}
	defer root.Close()

	var prefix string
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		prefix = filepath.Join(prefix, part)
		info, err := root.Lstat(prefix)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return folderTarget{}, ErrFolderUnavailable
		}
	}
	parent, err := root.OpenRoot(filepath.Dir(rel))
	if err != nil {
		return folderTarget{}, ErrFolderUnavailable
	}
	name := filepath.Base(rel)
	dir, err := parent.OpenRoot(name)
	if err != nil {
		parent.Close()
		return folderTarget{}, ErrFolderUnavailable
	}
	info, err := dir.Stat(".")
	target := folderTarget{folder: f, lib: lib, parent: parent, dir: dir, info: info, name: name}
	if err != nil || !target.inPlace() {
		target.close()
		return folderTarget{}, ErrFolderUnavailable
	}
	return target, nil
}

func (f folderTarget) close() {
	if f.dir != nil {
		f.dir.Close()
	}
	f.parent.Close()
}

func (f folderTarget) inPlace() bool {
	info, err := f.parent.Lstat(f.name)
	return err == nil && info.IsDir() && os.SameFile(info, f.info)
}

func (f folderTarget) preview() (DeletePreview, []fs.FileInfo, error) {
	entries, err := fs.ReadDir(f.dir.FS(), ".")
	if err != nil {
		return DeletePreview{}, nil, err
	}
	preview := DeletePreview{Name: f.name}
	hash := sha256.New()
	fmt.Fprintln(hash, f.folder.ID)
	var files []fs.FileInfo
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || entry.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			preview.Subfolders++
			fmt.Fprintf(hash, "dir %q\n", name)
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if !videoExts[ext] && !imageExts[ext] && !subtitleExts[ext] && ext != ".vtt" && ext != ".sub" && ext != ".sup" {
			continue
		}
		info, err := f.dir.Lstat(name)
		if err != nil {
			return DeletePreview{}, nil, err
		}
		if info.Mode().IsRegular() {
			files = append(files, info)
			fmt.Fprintf(hash, "file %q %d %d\n", name, info.Size(), info.ModTime().UnixNano())
		}
	}
	preview.Files = len(files)
	preview.Token = hex.EncodeToString(hash.Sum(nil))
	return preview, files, nil
}

func (s *Store) PreviewFolderDeletion(id string) (DeletePreview, error) {
	target, err := s.openFolderTarget(id)
	if err != nil {
		return DeletePreview{}, err
	}
	defer target.close()
	if err := s.checkDeleteLocation(target); err != nil {
		return DeletePreview{}, err
	}
	preview, _, err := target.preview()
	return preview, err
}

func renamedFolder(name, reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > 80 || strings.HasSuffix(reason, ".") ||
		strings.ContainsAny(reason, "<>:\"/\\|?*") || strings.ContainsFunc(reason, unicode.IsControl) {
		return "", ErrDeleteReason
	}
	renamed := name + "-" + reason
	length := len(renamed)
	if runtime.GOOS == "windows" {
		length = len(utf16.Encode([]rune(renamed)))
	}
	if !filepath.IsLocal(renamed) || length > 255 {
		return "", fmt.Errorf("%w，改名后的文件夹名称太长或含有无效字符", ErrDeleteReason)
	}
	return renamed, nil
}

func (s *Store) checkDeleteLocation(target folderTarget) error {
	path, err := filepath.EvalSymlinks(filepath.Join(target.lib.Path, filepath.FromSlash(target.folder.RelPath)))
	if err != nil {
		return ErrFolderUnavailable
	}
	if pathsOverlap(path, s.dataDir) {
		return fmt.Errorf("%w：文件夹与 Hestia 数据目录重叠", ErrDeleteConflict)
	}
	for _, lib := range s.Libraries() {
		if lib.ID != target.lib.ID && pathsOverlap(path, lib.Path) {
			return fmt.Errorf("%w：文件夹与另一个媒体库重叠", ErrDeleteConflict)
		}
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	if resolved, err := filepath.EvalSymlinks(b); err == nil {
		b = resolved
	}
	if abs, err := filepath.Abs(b); err == nil {
		b = abs
	}
	return containsPath(a, b) || containsPath(b, a)
}

func containsPath(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && filepath.IsLocal(rel)
}

func destinationAvailable(parent *os.Root, name string) error {
	_, err := parent.Lstat(name)
	if err == nil {
		return fmt.Errorf("%w：同名文件夹或文件已存在", ErrDeleteConflict)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store) DeleteFolderMedia(id, reason, token string, prepare func(folderPath string, folderIDs []string) (func(), error)) (FolderDeletion, error) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()

	var release func()
	defer func() {
		if release != nil {
			release()
		}
	}()
	result, err := s.deleteFolderMedia(id, reason, token, func(folderPath string, folderIDs []string) error {
		var err error
		release, err = prepare(folderPath, folderIDs)
		return err
	})
	if err != nil {
		return FolderDeletion{}, err
	}
	if err := s.rescanLibrary(result.libraryID); err != nil {
		if result.Warning != "" {
			result.Warning += "。"
		}
		result.Warning += "索引刷新失败，请在控制台重新扫描"
	}
	return result, nil
}

func (s *Store) deleteFolderMedia(id, reason, token string, prepare func(folderPath string, folderIDs []string) error) (FolderDeletion, error) {
	s.libraryMu.Lock()
	defer s.libraryMu.Unlock()

	target, err := s.openFolderTarget(id)
	if err != nil {
		return FolderDeletion{}, err
	}
	defer func() { target.close() }()
	name, err := renamedFolder(target.name, reason)
	if err != nil {
		return FolderDeletion{}, err
	}
	if err := s.checkDeleteLocation(target); err != nil {
		return FolderDeletion{}, err
	}
	preview, files, err := target.preview()
	if err != nil {
		return FolderDeletion{}, err
	}
	if preview.Token != token {
		return FolderDeletion{}, fmt.Errorf("%w：文件夹内容已变化，请重新确认", ErrDeleteConflict)
	}
	if len(files) == 0 {
		return FolderDeletion{}, fmt.Errorf("%w：当前文件夹没有可删除的媒体", ErrDeleteConflict)
	}
	if err := destinationAvailable(target.parent, name); err != nil {
		return FolderDeletion{}, err
	}
	if err := prepare(target.dir.Name(), s.folderTreeIDs(target.folder)); err != nil {
		return FolderDeletion{}, err
	}
	confirmedFiles := files
	preview, files, err = target.preview()
	if err != nil {
		return FolderDeletion{}, err
	}
	if preview.Token != token {
		return FolderDeletion{}, fmt.Errorf("%w：文件夹内容已变化，请重新确认", ErrDeleteConflict)
	}
	for i, file := range files {
		if !os.SameFile(file, confirmedFiles[i]) {
			return FolderDeletion{}, fmt.Errorf("%w：文件夹内容已变化，请重新确认", ErrDeleteConflict)
		}
	}

	// Move only the confirmed files out of the folder so a failed rename can be
	// rolled back before any of them are removed.
	stage := ".hestia-delete-" + rand.Text()
	if err := target.dir.Mkdir(stage, 0o700); err != nil {
		return FolderDeletion{}, err
	}
	stagingDir, err := target.dir.OpenRoot(stage)
	if err != nil {
		_ = target.dir.Remove(stage)
		return FolderDeletion{}, err
	}
	defer stagingDir.Close()
	stageInfo, err := stagingDir.Stat(".")
	if err != nil {
		return FolderDeletion{}, err
	}
	stageInPlace := func() bool {
		info, err := target.dir.Lstat(stage)
		return err == nil && info.IsDir() && os.SameFile(info, stageInfo)
	}
	staged := make([]string, 0, len(files))
	rollback := func(cause error) (FolderDeletion, error) {
		if !stageInPlace() {
			return FolderDeletion{}, fmt.Errorf("临时目录已移动，请检查 %s：%w", stagingDir.Name(), cause)
		}
		stagingDir.Close()
		return FolderDeletion{}, target.restore(stage, staged, cause)
	}
	for _, file := range files {
		if err := verifyDeleteFile(target.dir, file.Name(), file); err != nil {
			return rollback(err)
		}
		if !stageInPlace() {
			return rollback(ErrDeleteConflict)
		}
		if err := target.dir.Rename(file.Name(), filepath.Join(stage, file.Name())); err != nil {
			return rollback(err)
		}
		staged = append(staged, file.Name())
		if err := verifyDeleteFile(stagingDir, file.Name(), file); err != nil {
			return rollback(err)
		}
	}
	if !target.inPlace() || !stageInPlace() {
		return rollback(ErrDeleteConflict)
	}
	if err := destinationAvailable(target.parent, name); err != nil {
		return rollback(err)
	}
	// Windows disallows renaming a directory while these root handles are open.
	stagingDir.Close()
	target.dir.Close()
	target.dir = nil
	if err := target.parent.Rename(target.name, name); err != nil {
		if openErr := target.reopen(target.name); openErr != nil {
			return FolderDeletion{}, fmt.Errorf("无法改名，请检查 %s：%w", stagingDir.Name(), err)
		}
		return rollback(err)
	}

	newRel := filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(target.folder.RelPath)), name))
	result := FolderDeletion{ID: NewID(target.lib.Path, newRel), Name: name, Files: len(files), libraryID: target.lib.ID}
	s.folderProgressChanges(target, newRel, &result)

	stagePath := filepath.Join(target.parent.Name(), name, stage)
	if err := target.reopen(name); err != nil || !stageInPlace() {
		result.Files = 0
		result.Warning = "文件夹已改名，请检查临时目录 " + stagePath
		return result, nil
	}
	stagingDir, err = target.dir.OpenRoot(stage)
	if err != nil {
		result.Files = 0
		result.Warning = "文件夹已改名，请检查临时目录 " + stagePath
		return result, nil
	}
	reopened, err := stagingDir.Stat(".")
	if err != nil || !os.SameFile(reopened, stageInfo) {
		stagingDir.Close()
		result.Files = 0
		result.Warning = "文件夹已改名，请检查临时目录 " + stagePath
		return result, nil
	}
	for _, file := range files {
		if err := verifyDeleteFile(stagingDir, file.Name(), file); err != nil {
			result.Files--
			continue
		}
		if err := stagingDir.Remove(file.Name()); err != nil {
			result.Files--
		}
	}
	stagingDir.Close()
	if result.Files != len(files) || !stageInPlace() || target.dir.Remove(stage) != nil {
		result.Warning = "文件夹已改名，请检查临时目录 " + stagePath
	}
	return result, nil
}

func (s *Store) folderTreeIDs(root Folder) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := []string{root.ID}
	for _, folder := range s.folders {
		if folder.LibraryID == root.LibraryID && strings.HasPrefix(folder.RelPath, root.RelPath+"/") {
			ids = append(ids, folder.ID)
		}
	}
	return ids
}

func (f *folderTarget) reopen(name string) error {
	dir, err := f.parent.OpenRoot(name)
	if err != nil {
		return err
	}
	info, err := dir.Stat(".")
	if err != nil || !os.SameFile(info, f.info) {
		dir.Close()
		return ErrDeleteConflict
	}
	f.dir = dir
	f.name = name
	if !f.inPlace() {
		dir.Close()
		f.dir = nil
		return ErrDeleteConflict
	}
	return nil
}

func verifyDeleteFile(parent *os.Root, name string, confirmed fs.FileInfo) error {
	info, err := parent.Lstat(name)
	if err != nil {
		return err
	}
	if !os.SameFile(info, confirmed) || !info.Mode().IsRegular() ||
		info.Size() != confirmed.Size() || !info.ModTime().Equal(confirmed.ModTime()) {
		return fmt.Errorf("%w：文件夹内容已变化，请重新确认", ErrDeleteConflict)
	}
	return nil
}

func (f folderTarget) restore(stage string, files []string, cause error) error {
	var failed bool
	for _, file := range files {
		src := filepath.Join(stage, file)
		// Link fails if another process has recreated the original name.
		if err := f.dir.Link(src, file); err != nil {
			failed = true
			continue
		}
		if err := f.dir.Remove(src); err != nil {
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("操作中断，部分文件需要从 %s 恢复：%w", filepath.Join(f.dir.Name(), stage), cause)
	}
	_ = f.dir.Remove(stage)
	return cause
}

func (s *Store) folderProgressChanges(target folderTarget, newRel string, result *FolderDeletion) {
	result.Moved = make(map[string]string)
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefix := target.folder.RelPath + "/"
	for _, media := range s.media {
		if media.LibraryID != target.lib.ID || media.Kind != KindVideo {
			continue
		}
		suffix, ok := strings.CutPrefix(media.RelPath, prefix)
		if !ok {
			continue
		}
		if strings.Contains(suffix, "/") {
			result.Moved[media.ID] = NewID(target.lib.Path, newRel+"/"+suffix)
		} else {
			result.Removed = append(result.Removed, media.ID)
		}
	}
}
