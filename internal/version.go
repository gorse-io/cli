// Copyright 2026 gorse Project Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"fmt"
	"runtime"
)

// Default build-time variable.
// These values are overridden via ldflags
var (
	Version    = "unknown-version"
	GitCommit  = "unknown-commit"
	BuildTime  = "unknown-buildtime"
	APIVersion = "v0.2.7"
)

func BuildInfo() string {
	var buildInfo string
	buildInfo += fmt.Sprintln("Version:\t", Version)
	buildInfo += fmt.Sprintln("API version:\t", APIVersion)
	buildInfo += fmt.Sprintln("Go version:\t", runtime.Version())
	buildInfo += fmt.Sprintln("Git commit:\t", GitCommit)
	buildInfo += fmt.Sprintln("Built:\t\t", BuildTime)
	buildInfo += fmt.Sprintf("OS/Arch:\t %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return buildInfo
}
