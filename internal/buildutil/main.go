// Command buildutil configures the shared development dashboards for this repo.
package main

import (
	"os"
	"time"

	buildutil "github.com/Obedience-Corp/build-util"
)

func main() {
	buildutil.Run(os.Args[1:], buildutil.BuildConfig{
		BinaryName:  "agent-stream-dbg",
		MainPath:    "./cmd/agent-stream-dbg",
		SectionName: "agent-stream-dbg",
		TestTimeout: 2 * time.Minute,
	})
}
