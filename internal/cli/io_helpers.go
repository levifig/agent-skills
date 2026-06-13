package cli

import "io"

func firstReader(readers ...io.Reader) io.Reader {
	for _, reader := range readers {
		if reader != nil {
			return reader
		}
	}
	return nil
}

func firstWriter(writers ...io.Writer) io.Writer {
	for _, writer := range writers {
		if writer != nil {
			return writer
		}
	}
	return nil
}
