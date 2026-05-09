// Package diskmon 提供数据目录所在分区的空间统计及 SQLite/日志目录占用估算。
package diskmon

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// FreeAndTotalBytes 对 path 所在分区执行 statfs，返回可用字节数与总容量字节数。
func FreeAndTotalBytes(path string) (free uint64, total uint64, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	dir := filepath.Dir(abs)
	var st syscall.Statfs_t
	if err = syscall.Statfs(dir, &st); err != nil {
		return 0, 0, err
	}
	free = uint64(st.Bavail) * uint64(st.Bsize)
	total = uint64(st.Blocks) * uint64(st.Bsize)
	return free, total, nil
}

// SQLiteBundleSize 估算主库文件及常见附属文件（-wal、-shm、-journal）占用字节之和。
func SQLiteBundleSize(dbPath string) int64 {
	var sum int64
	candidates := []string{
		dbPath,
		dbPath + "-wal",
		dbPath + "-shm",
		dbPath + "-journal",
	}
	for _, p := range candidates {
		fi, err := os.Stat(p)
		if err != nil || fi.IsDir() {
			continue
		}
		sum += fi.Size()
	}
	return sum
}

// LogDirLogFilesSize 统计目录下直接子级 *.log 文件总字节数（不含子目录）。
func LogDirLogFilesSize(logDir string) int64 {
	logDir = strings.TrimSpace(logDir)
	if logDir == "" {
		return 0
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return 0
	}
	var sum int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".log") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		sum += fi.Size()
	}
	return sum
}
