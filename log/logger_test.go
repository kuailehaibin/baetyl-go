package log

import (
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	log := With(Any("height", "122"))
	log.Info("test")

	dir, err := ioutil.TempDir("", t.Name())
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	jsonFile := path.Join(dir, "json.log")
	cfg := Config{
		Filename:   jsonFile,
		Level:      "info",
		Encoding:   "json",
		MaxAge:     15,
		MaxSize:    1,
		MaxBackups: 15,
	}

	log, err = Init(cfg)
	assert.NoError(t, err)
	log.Info("baetyl", Any("age", 12), Error(errors.New("custom error")), Any("icon", "baetyl"), Any("duration", time.Duration(1)))
	log.Sync()
	assert.FileExists(t, jsonFile)

	bytes, err := ioutil.ReadFile(jsonFile)
	assert.NoError(t, err)
	res, _ := regexp.MatchString(`{"level":"info","ts":[0-9T:\.]+,"caller":".*logger_test.*","msg":"baetyl","age":12,"error":"custom error","errorVerbose":".*logger_test.*","icon":"baetyl","duration":.*}`, string(bytes))
	assert.True(t, res)

	log.Error("test error")
	log.Sync()
	bytes, err = ioutil.ReadFile(jsonFile)
	assert.NoError(t, err)
	res, _ = regexp.MatchString(`{"level":"error","ts":[0-9T:\.]+,"caller":".*logger_test.*","msg":"test error","stacktrace":".*"}`, string(bytes))
	assert.True(t, res)

	log.Debug("baetyl")
	log.Sync()
	bytes, err = ioutil.ReadFile(jsonFile)
	assert.NoError(t, err)
	assert.NotContains(t, string(bytes), `"level":"debug"`)

	log = With(Any("name", "baetyl"))
	log.Info("baetyl")

	bytes, err = ioutil.ReadFile(jsonFile)
	assert.NoError(t, err)
	res, _ = regexp.MatchString(`{"level":"info","ts":[0-9T:\.]+,"caller":".*logger_test.*","msg":"baetyl","name":"baetyl"}`, string(bytes))
	assert.True(t, res)

	cfg.Level = "xxx"
	log, err = Init(cfg)
	assert.NoError(t, err)
	log.Debug("baetyl")
	log.Sync()
	bytes, err = ioutil.ReadFile(jsonFile)
	assert.NoError(t, err)
	assert.NotContains(t, string(bytes), `"level":"debug"`)

	log, err = Init(cfg, Any("height", "122"))
	assert.NoError(t, err)
	assert.NotEmpty(t, log)
	log.Info("baetyl")
	log.Sync()
	bytes, err = ioutil.ReadFile(jsonFile)
	assert.NoError(t, err)
	res, _ = regexp.MatchString(`{"level":"info","ts":[0-9T:\.]+,"caller":".*logger_test.*","msg":"baetyl","height":"122"}`, string(bytes))
	assert.True(t, res)

	textFile := path.Join(dir, "console.log")
	cfg.Encoding = "console"
	cfg.Filename = textFile
	cfg.Level = "info"
	log, err = Init(cfg)
	assert.NoError(t, err)

	log.Info("baetyl")
	log.Sync()
	assert.FileExists(t, textFile)

	bytes, err = ioutil.ReadFile(textFile)
	assert.NoError(t, err)
	assert.Contains(t, string(bytes), "info")

	log.Debug("baetyl")
	log.Sync()
	bytes, err = ioutil.ReadFile(textFile)
	assert.NoError(t, err)
	assert.NotContains(t, string(bytes), "debug")

	log = With(Any("name", "baetyl"))
	log.Info("baetyl")
	bytes, err = ioutil.ReadFile(textFile)
	assert.NoError(t, err)
	assert.Contains(t, string(bytes), `{"name": "baetyl"}`)

	cfg.Level = "xxx"
	log, err = Init(cfg)
	assert.NoError(t, err)
	log.Debug("baetyl")
	log.Sync()
	bytes, err = ioutil.ReadFile(textFile)
	assert.NoError(t, err)
	assert.NotContains(t, string(bytes), "debug")

	log, err = Init(cfg, Any("height", "122"))
	assert.NoError(t, err)
	log.Info("baetyl")
	log.Sync()
	bytes, err = ioutil.ReadFile(textFile)
	assert.NoError(t, err)
	assert.Contains(t, string(bytes), `{"height": "122"}`)
}

func TestParseLevel(t *testing.T) {
	level := parseLevel("fatal")
	assert.Equal(t, FatalLevel, level)

	level = parseLevel("panic")
	assert.Equal(t, PanicLevel, level)

	level = parseLevel("error")
	assert.Equal(t, ErrorLevel, level)

	level = parseLevel("warn")
	assert.Equal(t, WarnLevel, level)

	level = parseLevel("warning")
	assert.Equal(t, WarnLevel, level)

	level = parseLevel("info")
	assert.Equal(t, InfoLevel, level)

	level = parseLevel("debug")
	assert.Equal(t, DebugLevel, level)

	level = parseLevel("xxx")
	assert.Equal(t, InfoLevel, level)
}

func TestNewFileHook(t *testing.T) {
	cfg := Config{
		Filename:   "&name=chen&log=wang",
		Compress:   true,
		MaxAge:     12,
		MaxSize:    13,
		MaxBackups: 14,
	}
	url := url.URL{
		Scheme:   "lumberjack",
		RawQuery: cfg.String(),
	}
	lumber, err := newFileHook(&url)
	assert.NoError(t, err)
	assert.True(t, lumber.(*lumberjackSink).Compress)
	assert.Equal(t, cfg.Filename, lumber.(*lumberjackSink).Filename)
	assert.Equal(t, 12, lumber.(*lumberjackSink).MaxAge)
	assert.Equal(t, 13, lumber.(*lumberjackSink).MaxSize)
	assert.Equal(t, 14, lumber.(*lumberjackSink).MaxBackups)
}

func BenchmarkConsoleAndFile(b *testing.B) {
	dir, err := ioutil.TempDir("", b.Name())
	assert.NoError(b, err)
	defer os.RemoveAll(dir)

	file := path.Join(dir, "test.log")
	cfg := Config{
		Filename:   file,
		Level:      "info",
		Encoding:   "json",
		MaxAge:     15,
		MaxSize:    1000,
		MaxBackups: 15,
	}
	logger, err := Init(cfg)
	assert.NoError(b, err)
	b.ResetTimer()
	b.Run("log", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Info("test: " + strconv.Itoa(i))
		}
		logger.Sync()
	})
}
