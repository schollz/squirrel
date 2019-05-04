package get

import (
	"bytes"
	"io"
	"os"
)

func countLines(fname string) (lines int, err error) {
	f, err := os.Open(fname)
	if err != nil {
		return
	}
	lines, err = lineCounter(f)
	return
}

func lineCounter(r io.Reader) (int, error) {
	buf := make([]byte, 32*1024)
	count := 1
	lineSep := []byte{'\n'}

	for {
		c, err := r.Read(buf)
		count += bytes.Count(buf[:c], lineSep)

		switch {
		case err == io.EOF:
			return count, nil

		case err != nil:
			return count, err
		}
	}
}
