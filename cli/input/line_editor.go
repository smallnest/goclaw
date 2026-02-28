package input

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

var ErrInterrupt = errors.New("interrupt")

type lineEditor struct {
	fd     int
	reader *bufio.Reader
	state  *unix.Termios
	prompt string

	buf    []rune
	cursor int

	history []string
	histPos int
	draft   []rune

	pasteMode    bool
	pasteBuilder strings.Builder
	pendingPaste string
	tabHandler   func() string
}

func NewLineEditor(prompt string, tabHandler func() string) (*lineEditor, error) {
	fd := int(os.Stdin.Fd())
	st, err := makeRaw(fd)
	if err != nil {
		return nil, err
	}
	e := &lineEditor{
		fd:         fd,
		reader:     bufio.NewReader(os.Stdin),
		state:      st,
		prompt:     prompt,
		histPos:    0,
		tabHandler: tabHandler,
	}
	e.histPos = len(e.history)
	e.enableBracketedPaste()
	return e, nil
}

// Suspend 暂时退出原始模式，用于输出多行内容
func (e *lineEditor) Suspend() error {
	// 清除当前行并换行
	_, _ = os.Stdout.WriteString("\r\n")
	// 退出原始模式
	return restoreTerminal(e.fd, e.state)
}

// Resume 恢复原始模式
func (e *lineEditor) Resume() error {
	// 重新进入原始模式
	_, err := makeRaw(e.fd)
	if err != nil {
		return err
	}
	// 重新启用 bracketed paste
	e.enableBracketedPaste()
	return nil
}

func (e *lineEditor) Close() error {
	e.disableBracketedPaste()
	_, _ = os.Stdout.WriteString("\r\n")
	return restoreTerminal(e.fd, e.state)
}

func (e *lineEditor) InitHistory(history []string) {
	e.history = e.history[:0]
	for _, h := range history {
		if h != "" {
			e.history = append(e.history, h)
		}
	}
	e.histPos = len(e.history)
}

func (e *lineEditor) SaveToHistory(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	if len(e.history) > 0 && e.history[len(e.history)-1] == line {
		return
	}
	e.history = append(e.history, line)
	e.histPos = len(e.history)
}

func (e *lineEditor) Refresh() {
	e.render()
}

func (e *lineEditor) ReadLine() (string, error) {
	e.render()
	for {
		k, r, err := e.readKey()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return "", io.EOF
			}
			return "", err
		}

		switch k {
		case keyCtrlC:
			return "", ErrInterrupt
		case keyCtrlD:
			if len(e.buf) == 0 {
				return "", io.EOF
			}
			if e.cursor < len(e.buf) {
				e.buf = append(e.buf[:e.cursor], e.buf[e.cursor+1:]...)
				e.render()
			}
			continue
		case keyTab:
			if e.tabHandler != nil {
				cmd := e.tabHandler()
				if cmd != "" {
					e.buf = []rune(cmd)
					e.cursor = len(e.buf)
					e.render()
				}
			}
			continue
		case keyEnter:
			if e.pasteMode {
				e.pasteBuilder.WriteRune('\n')
				continue
			}
			line := string(e.buf)
			if e.pendingPaste != "" && line == e.pastePlaceholder() {
				line = e.pendingPaste
				e.pendingPaste = ""
				_, _ = os.Stdout.WriteString("\r\n" + line + "\r\n")
			} else {
				_, _ = os.Stdout.WriteString("\r\n")
			}
			e.buf = e.buf[:0]
			e.cursor = 0
			e.histPos = len(e.history)
			e.draft = e.draft[:0]
			return line, nil
		case keyBackspace:
			if e.cursor > 0 {
				e.buf = append(e.buf[:e.cursor-1], e.buf[e.cursor:]...)
				e.cursor--
				e.render()
			}
		case keyLeft:
			if e.cursor > 0 {
				e.cursor--
				e.render()
			}
		case keyRight:
			if e.cursor < len(e.buf) {
				e.cursor++
				e.render()
			}
		case keyUp:
			e.historyUp()
			e.render()
		case keyDown:
			e.historyDown()
			e.render()
		case keyPasteStart:
			e.pasteMode = true
			e.pasteBuilder.Reset()
		case keyPasteEnd:
			e.pasteMode = false
			chunk := normalizePastedContent(e.pasteBuilder.String())
			e.pasteBuilder.Reset()
			if chunk != "" {
				newlines := strings.Count(chunk, "\n")
				if newlines > 0 {
					clearLines(newlines)
				}
				if e.pendingPaste == "" {
					e.pendingPaste = chunk
				} else {
					e.pendingPaste += "\n" + chunk
				}
				e.buf = []rune(e.pastePlaceholder())
				e.cursor = len(e.buf)
				e.render()
			}
		case keyRune:
			if e.pasteMode {
				e.pasteBuilder.WriteRune(r)
				continue
			}
			if e.pendingPaste != "" {
				// If user starts typing, cancel the pending pasted block.
				e.pendingPaste = ""
				e.buf = e.buf[:0]
				e.cursor = 0
			}
			e.insertRune(r)
			e.render()
		case keyUnknown:
			if e.pasteMode {
				e.pasteBuilder.WriteRune(r)
			}
		}
	}
}

func (e *lineEditor) insertRune(r rune) {
	if e.cursor == len(e.buf) {
		e.buf = append(e.buf, r)
		e.cursor++
		return
	}
	e.buf = append(e.buf, 0)
	copy(e.buf[e.cursor+1:], e.buf[e.cursor:])
	e.buf[e.cursor] = r
	e.cursor++
}

func (e *lineEditor) historyUp() {
	if len(e.history) == 0 {
		return
	}
	if e.histPos == len(e.history) {
		e.draft = append(e.draft[:0], e.buf...)
	}
	if e.histPos > 0 {
		e.histPos--
		e.buf = []rune(e.history[e.histPos])
		e.cursor = len(e.buf)
	}
}

func (e *lineEditor) historyDown() {
	if len(e.history) == 0 {
		return
	}
	if e.histPos < len(e.history)-1 {
		e.histPos++
		e.buf = []rune(e.history[e.histPos])
		e.cursor = len(e.buf)
		return
	}
	if e.histPos == len(e.history)-1 {
		e.histPos = len(e.history)
		e.buf = append(e.buf[:0], e.draft...)
		e.cursor = len(e.buf)
	}
}

func (e *lineEditor) render() {
	line := string(e.buf)
	_, _ = os.Stdout.WriteString("\r\x1b[2K" + e.prompt + line)
	tail := len(e.buf) - e.cursor
	if tail > 0 {
		_, _ = os.Stdout.WriteString(fmt.Sprintf("\x1b[%dD", tail))
	}
}

func (e *lineEditor) ClearCurrentLine() {
	_, _ = os.Stdout.WriteString("\r\x1b[2K")
}

func normalizePastedContent(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimRight(s, "\n")
}

func clearLines(count int) {
	if count <= 0 {
		return
	}
	fmt.Printf("\x1b[%dA\x1b[J", count)
}

func (e *lineEditor) pastePlaceholder() string {
	return fmt.Sprintf("[Pasted Content %d chars]", utf8.RuneCountInString(e.pendingPaste))
}

func (e *lineEditor) enableBracketedPaste() {
	_, _ = os.Stdout.WriteString("\x1b[?2004h")
}

func (e *lineEditor) disableBracketedPaste() {
	_, _ = os.Stdout.WriteString("\x1b[?2004l")
}

type keyType int

const (
	keyUnknown keyType = iota
	keyRune
	keyEnter
	keyBackspace
	keyCtrlC
	keyCtrlD
	keyLeft
	keyRight
	keyUp
	keyDown
	keyPasteStart
	keyPasteEnd
	keyTab
)

func (e *lineEditor) readKey() (keyType, rune, error) {
	r, _, err := e.reader.ReadRune()
	if err != nil {
		return keyUnknown, 0, err
	}

	switch r {
	case 3:
		return keyCtrlC, 0, nil
	case 4:
		return keyCtrlD, 0, nil
	case '\r', '\n':
		return keyEnter, 0, nil
	case 127, 8:
		return keyBackspace, 0, nil
	case '\t':
		return keyTab, 0, nil
	case 0x1b:
		return e.readEscapeKey()
	default:
		return keyRune, r, nil
	}
}

func (e *lineEditor) readEscapeKey() (keyType, rune, error) {
	r, _, err := e.reader.ReadRune()
	if err != nil {
		return keyUnknown, 0, err
	}
	if r != '[' {
		return keyUnknown, 0, nil
	}

	var sb strings.Builder
	for {
		rn, _, err := e.reader.ReadRune()
		if err != nil {
			return keyUnknown, 0, err
		}
		sb.WriteRune(rn)
		if (rn >= 'A' && rn <= 'Z') || rn == '~' {
			break
		}
	}

	switch sb.String() {
	case "A":
		return keyUp, 0, nil
	case "B":
		return keyDown, 0, nil
	case "C":
		return keyRight, 0, nil
	case "D":
		return keyLeft, 0, nil
	case "200~":
		return keyPasteStart, 0, nil
	case "201~":
		return keyPasteEnd, 0, nil
	default:
		return keyUnknown, 0, nil
	}
}

func makeRaw(fd int) (*unix.Termios, error) {
	getReq, setReq := termiosRequests()
	termios, err := unix.IoctlGetTermios(fd, getReq)
	if err != nil {
		return nil, err
	}

	oldState := *termios
	// cfmakeraw equivalent
	termios.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	termios.Oflag &^= unix.OPOST
	termios.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	termios.Cflag &^= unix.CSIZE | unix.PARENB
	termios.Cflag |= unix.CS8
	termios.Cc[unix.VMIN] = 1
	termios.Cc[unix.VTIME] = 0

	if err := unix.IoctlSetTermios(fd, setReq, termios); err != nil {
		return nil, err
	}
	return &oldState, nil
}

func restoreTerminal(fd int, state *unix.Termios) error {
	_, setReq := termiosRequests()
	return unix.IoctlSetTermios(fd, setReq, state)
}
