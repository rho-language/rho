//
// @file deploy.go
// @brief Rho Deploy — put a built program on a host, and start it.
//
// A board is flashed; a host is deployed to. The RoboHub UNO Q is both: its
// Linux side is a Debian machine a program is copied to over SSH and run, and
// its MCU is written by a probe. This is the first of those. What a host needs
// to be reached is not a property of this program either — a login, a directory
// and a start command differ between two boards — so the target says: a spec
// carries a `deploy` block, and this reads it through rho-target and carries it
// out.
//
// It copies with rsync or scp, and it can carry a container image instead of a
// file: the image is built here, saved, copied, loaded and started on the
// target. Which is a deployment decision, so it is the spec's.
//
// Nothing here reaches a board in a test: the invocations are rendered by
// functions, and a test is a test of the rendering.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// ComponentName is this component's own name, which is what a module says to be
// reached as `rho deploy`.
const ComponentName = "rho-deploy"

// Deploy is the part of a target spec this program needs. It is read from what
// rho-target resolved, not from the spec file, so a spec that inherits arrives
// with the parts it did not write itself.
type Deploy struct {
	Method  string  `json:"method,omitempty"`
	Host    string  `json:"host,omitempty"`
	Port    int     `json:"port,omitempty"`
	User    string  `json:"user,omitempty"`
	Path    string  `json:"path,omitempty"`
	Run     string  `json:"run,omitempty"`
	Docker  *Docker `json:"docker,omitempty"`
	Comment string  `json:"comment,omitempty"`
}

// Docker describes deploying the program as a container image.
type Docker struct {
	File  string   `json:"file,omitempty"`
	Image string   `json:"image,omitempty"`
	Tag   string   `json:"tag,omitempty"`
	Run   []string `json:"run,omitempty"`
}

// declared reports whether the target says anything about deploying.
func (d Deploy) declared() bool {
	return strings.TrimSpace(d.Host) != "" || strings.TrimSpace(d.Path) != "" || d.Docker != nil
}

// target is where the SSH reaches: `user@host`, or `host`, or the local machine
// when the target names none — a toolchain that runs on the board deploys to
// itself.
func (d Deploy) target() string {
	host := strings.TrimSpace(d.Host)
	if host == "" {
		host = "localhost"
	}
	if user := strings.TrimSpace(d.User); user != "" {
		return user + "@" + host
	}
	return host
}

// remotePath is the full path an artefact lands at on the target.
func (d Deploy) remotePath(artifact string) string {
	return path.Join(strings.TrimSpace(d.Path), filepath.Base(artifact))
}

// copyInvocation renders how an artefact is copied to the target: the program
// and its arguments. rsync is the default because a redeploy of an unchanged
// tree is then almost free; scp is there for a target that has no rsync.
func (d Deploy) copyInvocation(artifact string) (string, []string, error) {
	if strings.TrimSpace(d.Path) == "" {
		return "", nil, fmt.Errorf("the target declares no deploy path to copy to")
	}
	remote := d.target() + ":" + d.remotePath(artifact)

	switch strings.TrimSpace(d.Method) {
	case "", "rsync":
		args := []string{"-az", "--mkpath"}
		if d.Port != 0 {
			args = append(args, "-e", "ssh -p "+strconv.Itoa(d.Port))
		}
		args = append(args, artifact, remote)
		return "rsync", args, nil
	case "scp":
		args := []string{"-r"}
		if d.Port != 0 {
			args = append(args, "-P", strconv.Itoa(d.Port))
		}
		args = append(args, artifact, remote)
		return "scp", args, nil
	default:
		return "", nil, fmt.Errorf("unknown deploy method %q: use rsync or scp", d.Method)
	}
}

// runInvocation renders how the copied program is started on the target. It is
// empty when the target says nothing about starting.
func (d Deploy) runInvocation(artifact string) (string, []string, bool) {
	if strings.TrimSpace(d.Run) == "" {
		return "", nil, false
	}
	command := strings.ReplaceAll(d.Run, "{artifact}", d.remotePath(artifact))
	args := []string{}
	if d.Port != 0 {
		args = append(args, "-p", strconv.Itoa(d.Port))
	}
	args = append(args, d.target(), command)
	return "ssh", args, true
}

// sshStep renders a shell command run on the target over SSH.
func (d Deploy) sshStep(command string) []string {
	args := []string{}
	if d.Port != 0 {
		args = append(args, "-p", strconv.Itoa(d.Port))
	}
	args = append(args, d.target(), command)
	return append([]string{"ssh"}, args...)
}

// dockerPlan renders the steps of a container deployment: build the image here,
// save it, carry it, load it and start it on the target. Each step is a program
// and its arguments.
func (d Deploy) dockerPlan(project, artifact string) ([][]string, error) {
	if d.Docker == nil {
		return nil, fmt.Errorf("the target declares no docker block")
	}
	if strings.TrimSpace(d.Path) == "" {
		return nil, fmt.Errorf("the target declares no deploy path to copy the image to")
	}
	file := strings.TrimSpace(d.Docker.File)
	if file == "" {
		file = "Dockerfile"
	}
	image := strings.TrimSpace(d.Docker.Image)
	if image == "" {
		return nil, fmt.Errorf("the docker block names no image")
	}
	tag := strings.TrimSpace(d.Docker.Tag)
	if tag == "" {
		tag = "latest"
	}
	reference := image + ":" + tag
	context := project
	if context == "" {
		context = "."
	}
	tarball := filepath.Join(context, "build", image+".tar")

	steps := [][]string{
		{"docker", "build", "-f", filepath.Join(context, file), "-t", reference, context},
		{"docker", "save", "-o", tarball, reference},
	}
	copyProgram, copyArgs, err := d.copyInvocation(tarball)
	if err != nil {
		return nil, err
	}
	steps = append(steps, append([]string{copyProgram}, copyArgs...))
	steps = append(steps, d.sshStep("docker load -i "+d.remotePath(tarball)))

	runArgs := make([]string, 0, len(d.Docker.Run)+1)
	for _, arg := range d.Docker.Run {
		runArgs = append(runArgs, strings.ReplaceAll(arg, "{image}", reference))
	}
	if len(runArgs) == 0 {
		runArgs = append(runArgs, reference)
	}
	steps = append(steps, d.sshStep("docker run -d "+strings.Join(runArgs, " ")))
	return steps, nil
}

/* -------------------------------------------------------------------------- */
/*  Resolution                                                                 */
/* -------------------------------------------------------------------------- */

// resolve asks rho-target what a target is, and returns how to reach it. The
// specs are read where every build reads them, so a board is described once.
func resolve(name, project string) (Deploy, error) {
	if project == "" {
		project = "."
	}
	cmd := exec.Command("rho", "target", "show", "-json", "-project", project, name)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return Deploy{}, fmt.Errorf("rho target show %s: %w", name, err)
	}
	deploy, err := parseDeploy(out)
	if err != nil {
		return Deploy{}, fmt.Errorf("rho target show %s returned something that is not a target spec: %w", name, err)
	}
	return deploy, nil
}

// parseDeploy reads the deploy block out of a resolved target spec, as
// `rho target show -json` emits it.
func parseDeploy(data []byte) (Deploy, error) {
	var spec struct {
		Name   string `json:"name"`
		Deploy Deploy `json:"deploy"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		return Deploy{}, fmt.Errorf("not a target spec: %w", err)
	}
	return spec.Deploy, nil
}

// overrideFromEnv lets a machine redirect a deploy without editing a spec: the
// address and login of a board are facts about the bench, and a spec that
// carried them would be a spec that only worked on one machine.
func overrideFromEnv(d Deploy) Deploy {
	for _, pair := range []struct {
		name string
		assign func(string)
	}{
		{"RH_DEPLOY_HOST", func(v string) { d.Host = v }},
		{"RH_DEPLOY_USER", func(v string) { d.User = v }},
		{"RH_DEPLOY_PATH", func(v string) { d.Path = v }},
		{"RH_DEPLOY_METHOD", func(v string) { d.Method = v }},
	} {
		if v := strings.TrimSpace(os.Getenv(pair.name)); v != "" {
			pair.assign(v)
		}
	}
	if v := strings.TrimSpace(os.Getenv("RH_DEPLOY_PORT")); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			d.Port = port
		}
	}
	// The container facts — the image name, the tag, the Dockerfile and the run
	// arguments — are the project's, not the board's, so they can be redirected
	// here too: a board spec that carried one project's image name would be a
	// board spec that only worked for one project.
	dockerFile := strings.TrimSpace(os.Getenv("RH_DEPLOY_DOCKER_FILE"))
	dockerImage := strings.TrimSpace(os.Getenv("RH_DEPLOY_DOCKER_IMAGE"))
	dockerTag := strings.TrimSpace(os.Getenv("RH_DEPLOY_DOCKER_TAG"))
	dockerRun := strings.TrimSpace(os.Getenv("RH_DEPLOY_DOCKER_RUN"))
	if dockerFile != "" || dockerImage != "" || dockerTag != "" || dockerRun != "" {
		if d.Docker == nil {
			d.Docker = &Docker{}
		}
		if dockerFile != "" {
			d.Docker.File = dockerFile
		}
		if dockerImage != "" {
			d.Docker.Image = dockerImage
		}
		if dockerTag != "" {
			d.Docker.Tag = dockerTag
		}
		if dockerRun != "" {
			d.Docker.Run = strings.Fields(dockerRun)
		}
	}
	return d
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                   */
/* -------------------------------------------------------------------------- */

// commandRunner is the process runner, a variable so a test can see what would
// be run without running it.
var commandRunner = exec.Command

// runSteps runs each step in order, printing it first. A dry run prints and does
// not run, which is what makes a deployment inspectable before it touches a
// board.
func runSteps(steps [][]string, dryRun bool) error {
	for _, step := range steps {
		if len(step) == 0 {
			continue
		}
		fmt.Println(strings.Join(step, " "))
		if dryRun {
			continue
		}
		cmd := commandRunner(step[0], step[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s failed: %w", step[0], err)
		}
	}
	return nil
}

// Deploy puts a built artefact on a target and, when the target says how,
// starts it. `artifact` is a file or a directory to copy.
func DeployArtifact(artifact, targetName, project string, asDocker, start, dryRun bool) error {
	if strings.TrimSpace(artifact) == "" {
		return fmt.Errorf("nothing to deploy: name a file or directory")
	}
	if _, err := os.Stat(artifact); err != nil {
		return fmt.Errorf("nothing to deploy: %w", err)
	}

	deploy, err := resolve(targetName, project)
	if err != nil {
		return err
	}
	deploy = overrideFromEnv(deploy)
	if !deploy.declared() {
		return fmt.Errorf("the target %q declares no deploy block; add one to its spec", targetName)
	}
	if deploy.Comment != "" {
		fmt.Fprintf(os.Stderr, "note: %s\n", deploy.Comment)
	}

	if asDocker {
		steps, err := deploy.dockerPlan(project, artifact)
		if err != nil {
			return err
		}
		return runSteps(steps, dryRun)
	}

	program, args, err := deploy.copyInvocation(artifact)
	if err != nil {
		return err
	}
	steps := [][]string{append([]string{program}, args...)}
	if start {
		if program, args, ok := deploy.runInvocation(artifact); ok {
			steps = append(steps, append([]string{program}, args...))
		}
	}
	return runSteps(steps, dryRun)
}

/* -------------------------------------------------------------------------- */
/*  CLI                                                                        */
/* -------------------------------------------------------------------------- */

func main() {
	fs := flag.NewFlagSet("rho deploy", flag.ExitOnError)
	target := fs.String("target", "", "the target to deploy to: a board name from its spec")
	project := fs.String("project", "", "the project whose target to read (default: the working directory)")
	asDocker := fs.Bool("docker", false, "deploy as a container image")
	start := fs.Bool("run", false, "start the program on the target after copying")
	dryRun := fs.Bool("n", false, "print what would be run, and run nothing")

	// Flags are read wherever they appear, so `rho deploy app -target uno-q` is
	// what a person types first.
	argv := []string{}
	valueFlags := map[string]bool{"-target": true, "--target": true, "-project": true, "--project": true}
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case arg == "-h" || arg == "--help":
			usage()
			os.Exit(0)
		case valueFlags[arg] && i+1 < len(os.Args):
			argv = append(argv, arg, os.Args[i+1])
			i++
		default:
			argv = append(argv, arg)
		}
	}
	if err := fs.Parse(argv); err != nil {
		os.Exit(2)
	}

	if *target == "" {
		fmt.Fprintln(os.Stderr, "rho deploy: -target names the board to deploy to")
		usage()
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "rho deploy: name exactly one file or directory")
		usage()
		os.Exit(2)
	}

	if err := DeployArtifact(fs.Arg(0), *target, *project, *asDocker || envTruthy("RH_DEPLOY_DOCKER"), *start, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "rho deploy:", err)
		os.Exit(1)
	}
}

// envTruthy reports whether an environment variable is set to a yes.
func envTruthy(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: rho deploy -target <board> [-project dir] [-docker] [-run] [-n] <file|dir>")
	fmt.Fprintln(os.Stderr, "       the target's own spec says where it is reached and how")
}
