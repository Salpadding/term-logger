package main

// log key codes

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strconv"
	"sync"
	"syscall"
	"time"

	pty "github.com/creack/pty"
	"golang.org/x/term"
	te "golang.org/x/term"
)

var (
	_ io.Reader = (*TermLogger)(nil)
)

type config struct {
	Program []string      `json:"program"`
	Log     string        `json:"log"`
	Escapes []interface{} `json:"escapes"`
	Sync    int           `json:"sync"`
	Flags   struct {
		MakeRaw bool `json:"makeRaw"`
	} `json:"flags"`
}

func (c *config) Init() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	configPath := path.Join(home, ".config", "term-logger", "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(data, c)
	if err != nil {
		panic(err)
	}

}

type TermLogger struct {
	codes     [256]string
	config    *config
	mtx       sync.Mutex
	formatted []string
	logFile   *os.File
	ptmx      *os.File
}

func (c *config) NewLogger() *TermLogger {
	var err error
	term := TermLogger{
		config: c,
	}
	for i := 0; i < 256; i++ {
		if i < 128 && i >= 32 {
			term.codes[i] = string([]byte{byte(i)})
			continue
		}
		term.codes[i] = fmt.Sprintf("\\x%x", i)
	}

	for i := range c.Escapes {
		switch val := c.Escapes[i].(type) {
		case []interface{}:
			{
				for j := range val {
					fmt.Printf("map \\x%d -> %s", i*16+j, val[j])
					term.codes[i*16+j] = val[j].(string)
				}
			}
		case map[string]interface{}:
			{
				for k, v := range val {
					idx, _ := strconv.Atoi(k)
					fmt.Printf("map \\x%d -> %s", idx, v)
					term.codes[idx] = v.(string)
				}
			}
		}
	}

	if len(c.Log) > 0 {
		term.logFile, err = os.OpenFile(c.Log, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			panic(err)
		}
	}

	return &term
}

func (t *TermLogger) Read(p []byte) (n int, err error) {
	n, err = syscall.Read(0, p)
	if err != nil {
		return
	}

	t.mtx.Lock()
	defer t.mtx.Unlock()
	for i := 0; i < n; i++ {
		t.formatted = append(t.formatted, t.codes[p[i]])
	}
	return
}

func (t *TermLogger) Run() {
	var (
		cmd *exec.Cmd
		err error
	)
	if len(t.config.Program) == 0 {
		panic("missing program")
	}

	if t.config.Flags.MakeRaw {
		var old *te.State
		if _, err = te.MakeRaw(int(os.Stdin.Fd())); err != nil {
			panic(err)
		}
		defer func() { _ = term.Restore(int(os.Stdin.Fd()), old) }() // Best effort.
	}

	if len(t.config.Program) > 1 {
		cmd = exec.Command(t.config.Program[0], t.config.Program[1:]...)
	} else {
		cmd = exec.Command(t.config.Program[0])
	}
	t.ptmx, err = pty.Start(cmd)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = t.ptmx.Close()
	}()

	ch := make(chan os.Signal, 1)
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, t.ptmx); err != nil {
				log.Printf("error resizing pty: %s", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH                        // Initial resize.
	defer func() { signal.Stop(ch); close(ch) }() // Cleanup signals when done.

	go func() {
		ticker := time.NewTicker(time.Duration(t.config.Sync) * time.Second)

		for range ticker.C {
			if !t.mtx.TryLock() {
				continue
			}

			for _, s := range t.formatted {
				t.logFile.Write([]byte(s))
			}
			t.logFile.Sync()
			t.formatted = nil
			t.mtx.Unlock()
		}
	}()

	go func() {
		io.Copy(os.Stdout, t.ptmx)
	}()
	io.Copy(t.ptmx, t)
}

func main() {
	var config config
	config.Init()
	term := config.NewLogger()
	term.Run()
}
