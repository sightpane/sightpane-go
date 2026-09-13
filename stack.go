package sightpane

import (
	"fmt"
	"runtime"
	"strings"
)

// captureStack parses the current call stack, omitting internal runtime and SDK frames.
func captureStack(skip int) ([]Frame, string) {
	const maxFrames = 64
	rpc := make([]uintptr, maxFrames)
	n := runtime.Callers(skip+1, rpc)
	if n == 0 {
		return nil, ""
	}

	framesIter := runtime.CallersFrames(rpc[:n])
	var frames []Frame
	var b strings.Builder

	for {
		f, more := framesIter.Next()
		// Filter out internal runtime machinery
		if f.Function == "" {
			if !more {
				break
			}
			continue
		}

		// Skip sightpane internal frames
		if strings.Contains(f.Function, "sightpane/sightpane-go") &&
			!strings.Contains(f.Function, "_test") {
			if !more {
				break
			}
			continue
		}

		frame := Frame{
			Function: f.Function,
			Filename: f.File,
			Lineno:   f.Line,
		}
		frames = append(frames, frame)

		fmt.Fprintf(&b, "%s\n\t%s:%d\n", f.Function, f.File, f.Line)

		if !more {
			break
		}
	}

	return frames, b.String()
}
