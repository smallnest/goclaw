//go:build windows

package input

import (
	"errors"
)

var ErrInterrupt = errors.New("interrupt")

type lineEditor struct {
	fd     int
	prompt string
}

func NewLineEditor(prompt string, tabHandler func() string) (*lineEditor, error) {
	return nil, errors.New("line editor not supported on Windows")
}

func (e *lineEditor) Suspend() error {
	return nil
}

func (e *lineEditor) Resume() error {
	return nil
}

func (e *lineEditor) Close() error {
	return nil
}

func (e *lineEditor) InitHistory(history []string) {}

func (e *lineEditor) SaveToHistory(line string) {}

func (e *lineEditor) Refresh() {}

func (e *lineEditor) ReadLine() (string, error) {
	return "", errors.New("line editor not supported on Windows")
}

func (e *lineEditor) ClearCurrentLine() {}
