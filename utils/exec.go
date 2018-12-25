/*
 * Copyright 2018 The CovenantSQL Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package utils

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/CovenantSQL/CovenantSQL/utils/log"
)

// FJ is short for filepath.Join
var FJ = filepath.Join

// CMD is the struct holding exec.Cmd and log path
type CMD struct {
	Cmd     *exec.Cmd
	LogPath string
}

// GetProjectSrcDir gets the src code root
func GetProjectSrcDir() string {
	_, testFile, _, _ := runtime.Caller(0)
	return FJ(filepath.Dir(testFile), "../")
}

// Build runs build.sh
func Build() (err error) {
	wd := GetProjectSrcDir()
	err = os.Chdir(wd)
	if err != nil {
		log.WithError(err).Error("change working dir failed")
		return
	}
	cmd := exec.Command("./build.sh")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.WithError(err).Error("build failed")
	}
	log.Debugf("build output info: %#v", string(output))
	return
}

// RunCommand runs a command and capture its output to a log file,
//  if toStd is true also output to stdout and stderr
func RunCommand(bin string, args []string, processName string, workingDir string, logDir string, toStd bool) (err error) {
	cmd, err := RunCommandNB(bin, args, processName, workingDir, logDir, toStd)
	if err != nil {
		log.WithFields(log.Fields{
			"bin":     bin,
			"args":    args,
			"process": processName,
		}).WithError(err).Error("start command failed")
		return
	}
	err = cmd.Cmd.Wait()
	if err != nil {
		log.WithFields(log.Fields{
			"path": cmd.Cmd.Path,
			"args": cmd.Cmd.Args,
		}).WithError(err).Error("wait command failed")
		return
	}
	return
}

// RunCommandNB starts a non-blocking command
func RunCommandNB(bin string, args []string, processName string, workingDir string, logDir string, toStd bool) (cmd *CMD, err error) {
	cmd = new(CMD)
	cmd.LogPath = FJ(logDir, processName+".log")
	logFD, err := os.Create(cmd.LogPath)
	if err != nil {
		log.WithField("path", cmd.LogPath).WithError(err).Error("create log file failed")
		return
	}

	err = os.Chdir(workingDir)
	if err != nil {
		log.WithField("wd", workingDir).Error("change working dir failed")
		return
	}
	cmd.Cmd = exec.Command(bin, args...)

	if toStd {
		cmd.Cmd.Stdout = io.MultiWriter(os.Stdout, logFD)
		cmd.Cmd.Stderr = io.MultiWriter(os.Stderr, logFD)
	} else {
		cmd.Cmd.Stdout = logFD
		cmd.Cmd.Stderr = logFD
	}

	err = cmd.Cmd.Start()
	if err != nil {
		log.WithError(err).Error("cmd.Start() failed")
		return
	}

	return
}
