package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	maxAgeDays    = 7
	maxTotalBytes = 10 * 1024 * 1024
)

var defaultLogger = zap.New(newCore("", zapcore.DebugLevel), zap.AddCaller(), zap.AddCallerSkip(1))
var activeWriter = newDailyWriter("")

func Init(dir string) error {
	writer := newDailyWriter(dir)
	if err := writer.ensureFile(time.Now()); err != nil {
		return err
	}
	activeWriter = writer
	defaultLogger = zap.New(newCoreWithWriter(writer, zapcore.DebugLevel), zap.AddCaller(), zap.AddCallerSkip(1))
	return nil
}

func Info(format string, args ...any) {
	defaultLogger.Info(fmt.Sprintf(format, args...))
}

func Warn(format string, args ...any) {
	defaultLogger.Warn(fmt.Sprintf(format, args...))
}

func Error(format string, args ...any) {
	defaultLogger.Error(fmt.Sprintf(format, args...))
}

func InfoFields(message string, fields ...zap.Field) {
	defaultLogger.Info(message, fields...)
}

func WarnFields(message string, fields ...zap.Field) {
	defaultLogger.Warn(message, fields...)
}

func ErrorFields(message string, fields ...zap.Field) {
	defaultLogger.Error(message, fields...)
}

func Close() error {
	return defaultLogger.Sync()
}

func CurrentLog(maxBytes int64) (string, string, int64, error) {
	return activeWriter.CurrentLog(maxBytes)
}

func ClearCurrentLog() error {
	return activeWriter.ClearCurrentLog()
}

type core struct {
	writer *dailyWriter
	level  zapcore.LevelEnabler
	fields []zap.Field
}

func newCore(dir string, level zapcore.LevelEnabler) zapcore.Core {
	return newCoreWithWriter(newDailyWriter(dir), level)
}

func newCoreWithWriter(writer *dailyWriter, level zapcore.LevelEnabler) zapcore.Core {
	return &core{writer: writer, level: level}
}

func (c *core) Enabled(level zapcore.Level) bool {
	return c.level.Enabled(level)
}

func (c *core) With(fields []zap.Field) zapcore.Core {
	next := &core{
		writer: c.writer,
		level:  c.level,
		fields: make([]zap.Field, 0, len(c.fields)+len(fields)),
	}
	next.fields = append(next.fields, c.fields...)
	next.fields = append(next.fields, fields...)
	return next
}

func (c *core) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return checked.AddCore(entry, c)
	}
	return checked
}

func (c *core) Write(entry zapcore.Entry, fields []zap.Field) error {
	allFields := make([]zap.Field, 0, len(c.fields)+len(fields))
	allFields = append(allFields, c.fields...)
	allFields = append(allFields, fields...)

	line := fmt.Sprintf("[%s %s:%d] %s %s%s\n",
		entry.Time.Format("2006-01-02 15:04::05"),
		filepath.Base(entry.Caller.File),
		entry.Caller.Line,
		levelLabel(entry.Level),
		entry.Message,
		formatFields(allFields),
	)
	_, err := c.writer.Write([]byte(line))
	return err
}

func (c *core) Sync() error {
	return c.writer.Sync()
}

func levelLabel(level zapcore.Level) string {
	switch level {
	case zapcore.WarnLevel:
		return "Warn"
	case zapcore.ErrorLevel, zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		return "Error"
	default:
		return "Info"
	}
}

func formatFields(fields []zap.Field) string {
	if len(fields) == 0 {
		return ""
	}
	enc := zapcore.NewMapObjectEncoder()
	for _, field := range fields {
		field.AddTo(enc)
	}
	keys := make([]string, 0, len(enc.Fields))
	for key := range enc.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(" ")
	for i, key := range keys {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(fmt.Sprint(enc.Fields[key]))
	}
	return b.String()
}

type dailyWriter struct {
	mu      sync.Mutex
	dir     string
	file    *os.File
	day     string
	console *os.File
}

func newDailyWriter(dir string) *dailyWriter {
	return &dailyWriter{dir: strings.TrimSpace(dir), console: os.Stdout}
}

func (w *dailyWriter) Write(p []byte) (int, error) {
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.console != nil {
		_, _ = w.console.Write(p)
	}
	if w.dir == "" {
		return len(p), nil
	}
	if err := w.ensureFile(now); err != nil {
		return 0, err
	}
	w.enforceSizeLocked(int64(len(p)))
	return w.file.Write(p)
}

func (w *dailyWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

func (w *dailyWriter) CurrentLog(maxBytes int64) (string, string, int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	if err := w.ensureFile(now); err != nil {
		return "", "", 0, err
	}
	if w.file != nil {
		_ = w.file.Sync()
	}
	path := filepath.Join(w.dir, w.day+".log")
	info, err := os.Stat(path)
	if err != nil {
		return path, "", 0, err
	}
	size := info.Size()
	if maxBytes <= 0 || maxBytes > size {
		maxBytes = size
	}
	f, err := os.Open(path)
	if err != nil {
		return path, "", size, err
	}
	defer f.Close()
	if maxBytes < size {
		if _, err := f.Seek(size-maxBytes, 0); err != nil {
			return path, "", size, err
		}
	}
	buf := make([]byte, maxBytes)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return path, "", size, err
	}
	return path, string(buf[:n]), size, nil
}

func (w *dailyWriter) ClearCurrentLog() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ensureFile(time.Now()); err != nil {
		return err
	}
	if w.file == nil {
		return nil
	}
	if err := w.file.Truncate(0); err != nil {
		return err
	}
	_, err := w.file.Seek(0, 0)
	return err
}

func (w *dailyWriter) ensureFile(now time.Time) error {
	if w.dir == "" {
		return nil
	}
	day := now.Format("2006-01-02")
	if w.file != nil && w.day == day {
		return nil
	}
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return err
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	path := filepath.Join(w.dir, day+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.file = f
	w.day = day
	w.cleanupLocked(now)
	return nil
}

func (w *dailyWriter) cleanupLocked(now time.Time) {
	files := w.listLogFilesLocked()
	cutoff := now.AddDate(0, 0, -(maxAgeDays - 1))
	var total int64
	for _, file := range files {
		if file.day.Before(cutoff) {
			_ = os.Remove(file.path)
			continue
		}
		total += file.size
	}
	for _, file := range files {
		if total <= maxTotalBytes {
			break
		}
		if file.current || file.day.Before(cutoff) {
			continue
		}
		if err := os.Remove(file.path); err == nil {
			total -= file.size
		}
	}
}

func (w *dailyWriter) enforceSizeLocked(incoming int64) {
	files := w.listLogFilesLocked()
	var total int64
	for _, file := range files {
		total += file.size
	}
	for _, file := range files {
		if total+incoming <= maxTotalBytes {
			return
		}
		if file.current {
			continue
		}
		if err := os.Remove(file.path); err == nil {
			total -= file.size
		}
	}
	if total+incoming > maxTotalBytes && w.file != nil {
		_ = w.file.Truncate(0)
		_, _ = w.file.Seek(0, 0)
	}
}

type logFile struct {
	path    string
	day     time.Time
	size    int64
	current bool
}

func (w *dailyWriter) listLogFilesLocked() []logFile {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil
	}
	currentName := w.day + ".log"
	files := make([]logFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		dayText := strings.TrimSuffix(entry.Name(), ".log")
		day, err := time.ParseInLocation("2006-01-02", dayText, time.Local)
		if err != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, logFile{
			path:    filepath.Join(w.dir, entry.Name()),
			day:     day,
			size:    info.Size(),
			current: entry.Name() == currentName,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].day.Before(files[j].day)
	})
	return files
}
