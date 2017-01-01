package slogger

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

func TestLogType(t *testing.T) {
	assert := assert.New(t)

	log := NewLog(message.NewDefaultMessage(level.Info, "hello world"))
	plog := NewPrefixedLog("grip", message.NewDefaultMessage(level.Info, "hello world"))

	// this type must implement the composer interface.
	assert.Implements((*message.Composer)(nil), log)
	assert.Implements((*message.Composer)(nil), plog)

	// make sure that the constructors handle level setting
	assert.Equal(log.Priority(), level.Info)
	assert.Equal(plog.Priority(), level.Info)

	assert.True(log.Loggable())
	assert.True(plog.Loggable())

	// ensure that the resolve and the message handlers return different things.
	assert.True(strings.HasSuffix(log.Resolve(), "hello world\n"))
	assert.True(strings.HasSuffix(plog.Resolve(), "hello world\n"))
	assert.Equal("hello world", log.Message())
	assert.Equal("hello world", plog.Message())
	assert.NotEqual(log.Message(), log.Resolve())
	assert.NotEqual(plog.Message(), plog.Resolve())

	// prefixing should persist
	assert.Equal(log.Prefix, "")
	assert.Equal(plog.Prefix, "grip")
	assert.True(strings.Contains(plog.Resolve(), "grip"), fmt.Sprintf("%+v", log))
	assert.False(strings.Contains(log.Resolve(), "grip"), fmt.Sprintf("%+v", log))
	fmt.Println(fmt.Sprintf("%+v", log))
	assert.Equal(FormatLog(log), log.Resolve())
	assert.Equal(FormatLog(plog), plog.Resolve())

	for _, l := range []Level{WARN, INFO, DEBUG} {
		assert.NoError(log.SetPriority(l.Priority()))
		assert.Equal(l, convertFromPriority(log.Priority()))

		assert.NoError(plog.SetPriority(l.Priority()))
		assert.Equal(l, convertFromPriority(plog.Priority()))
	}

	assert.NotEqual(log.Raw(), plog.Raw())
	assert.True(len(fmt.Sprintf("%v", log.Raw())) > 10)
	assert.True(len(fmt.Sprintf("%v", plog.Raw())) > 10)

	assert.NoError(log.SetPriority(level.Info))
	assert.NoError(plog.SetPriority(level.Info))
}

func TestStripDirectories(t *testing.T) {
	assert := assert.New(t)

	paths := []string{"foo", "bar", "./foo", "./bar", "foo/bar", "bar/foo"}
	for _, path := range paths {
		assert.Equal(filepath.Base(path), stripDirectories(path, 2))
		assert.Equal(filepath.Base(path), stripDirectories(path, 1))
	}
}
