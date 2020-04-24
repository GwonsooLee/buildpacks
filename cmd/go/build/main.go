// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Implements go/build buildpack.
// The build buildpack runs go build.
package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/GoogleCloudPlatform/buildpacks/pkg/devmode"
	"github.com/GoogleCloudPlatform/buildpacks/pkg/env"
	gcp "github.com/GoogleCloudPlatform/buildpacks/pkg/gcpbuildpack"
	"github.com/GoogleCloudPlatform/buildpacks/pkg/golang"
	"github.com/buildpack/libbuildpack/layers"
)

const (
	cantLoadDefaultPackageError = "can't load package: package ."
)

func main() {
	gcp.Main(detectFn, buildFn)
}

func detectFn(ctx *gcp.Context) error {
	if !ctx.HasAtLeastOne("*.go") {
		ctx.OptOut("No *.go files found")
	}
	return nil
}

func buildFn(ctx *gcp.Context) error {
	// Create a cached layer for the GOCACHE.
	cl := ctx.Layer("gocache")
	lf := []layers.Flag{layers.Cache, layers.Build}
	if devmode.Enabled(ctx) {
		lf = append(lf, layers.Launch)
		ctx.OverrideLaunchEnv(cl, "GOCACHE", cl.Root)
	}
	ctx.WriteMetadata(cl, nil, lf...)

	// Create a layer for the compiled binary.  Add it to PATH in case
	// users wish to invoke the binary manually.
	bl := ctx.Layer("bin")
	ctx.PrependPathLaunchEnv(bl, "PATH", bl.Root)
	ctx.WriteMetadata(bl, nil, layers.Launch)
	outBin := filepath.Join(bl.Root, golang.OutBin)

	pkg, ok := os.LookupEnv(env.Buildable)
	if !ok {
		pkg = "."
	}

	// Build the application.
	ctx.ExecUserWithParams(gcp.ExecParams{
		Cmd: []string{"go", "build", "-o", outBin, pkg},
		Env: []string{"GOCACHE=" + cl.Root},
	}, printTipsAndKeepStderrTail(ctx))

	// Configure the entrypoint for production: use the full path to ease `skaffold debug`
	// as it does not access image contents.
	if !devmode.Enabled(ctx) {
		ctx.AddWebProcess([]string{outBin})
		return nil
	}

	// Configure the entrypoint and metadata for dev mode.
	devmode.AddFileWatcherProcess(ctx, devmode.Config{
		Cmd: []string{"go", "run", pkg},
		Ext: devmode.GoWatchedExtensions,
	})
	devmode.AddSyncMetadata(ctx, devmode.GoSyncRules)

	return nil
}

func printTipsAndKeepStderrTail(ctx *gcp.Context) gcp.ErrorSummaryProducer {
	return func(result *gcp.ExecResult) *gcp.Error {
		if result.ExitCode != 0 && strings.Contains(result.Stderr, cantLoadDefaultPackageError) {
			ctx.Tipf("Tip: %q env var configures which Go package is built. Default is '.'", env.Buildable)
		}

		return gcp.UserErrorKeepStderrTail(result)
	}
}
