//
// @file deploy_test.go
// @brief Rho Deploy tests.
//
// How a deployment is rendered, and what is refused. No board is involved: the
// copying and the starting are whatever the spec names, and a test that reached
// for a host would be testing the host.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"os/exec"
	"reflect"
	"testing"
)

// The default copy is rsync, and the artefact lands under the deploy path with
// its own name. A port becomes rsync's ssh transport argument, because a host
// and a port are two facts and the tool spells them two ways.
func TestCopyInvocationRsyncByDefault(t *testing.T) {
	deploy := Deploy{Host: "uno-q.local", Port: 2222, User: "arduino", Path: "/home/arduino/apps"}

	program, args, err := deploy.copyInvocation("build/gateway")
	if err != nil {
		t.Fatalf("copyInvocation: %v", err)
	}
	if program != "rsync" {
		t.Errorf("expected rsync, got %q", program)
	}
	want := []string{"-az", "--mkpath", "-e", "ssh -p 2222", "build/gateway",
		"arduino@uno-q.local:/home/arduino/apps/gateway"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("expected\n  %q\ngot\n  %q", want, args)
	}
}

// scp is there for a target that has no rsync, and spells the port its own way.
func TestCopyInvocationScp(t *testing.T) {
	deploy := Deploy{Method: "scp", Host: "uno-q", Port: 22, User: "arduino", Path: "/tmp/apps"}

	program, args, err := deploy.copyInvocation("gateway.tar")
	if err != nil {
		t.Fatalf("copyInvocation: %v", err)
	}
	if program != "scp" {
		t.Errorf("expected scp, got %q", program)
	}
	want := []string{"-r", "-P", "22", "gateway.tar", "arduino@uno-q:/tmp/apps/gateway.tar"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("expected\n  %q\ngot\n  %q", want, args)
	}
}

// A target with no path is one a deployment cannot be aimed at, and saying so is
// better than copying somewhere arbitrary.
func TestCopyWithoutAPathIsRefused(t *testing.T) {
	deploy := Deploy{Host: "uno-q"}
	if _, _, err := deploy.copyInvocation("gateway"); err == nil {
		t.Fatal("expected a target with no deploy path to be refused")
	}
}

func TestUnknownMethodIsRefused(t *testing.T) {
	deploy := Deploy{Method: "ftp", Host: "uno-q", Path: "/apps"}
	if _, _, err := deploy.copyInvocation("gateway"); err == nil {
		t.Fatal("expected an unknown method to be refused")
	}
}

// Starting is a command on the target with the artefact named as the spec wrote
// it, and it is reported as absent when the target says nothing about starting.
func TestRunInvocationSubstitutesTheArtifact(t *testing.T) {
	deploy := Deploy{Host: "uno-q", Port: 2222, User: "arduino", Path: "/home/arduino/apps",
		Run: "cd {artifact} && ./gateway"}

	program, args, ok := deploy.runInvocation("build/gateway")
	if !ok {
		t.Fatal("expected a run step")
	}
	if program != "ssh" {
		t.Errorf("expected ssh, got %q", program)
	}
	want := []string{"-p", "2222", "arduino@uno-q", "cd /home/arduino/apps/gateway && ./gateway"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("expected\n  %q\ngot\n  %q", want, args)
	}

	if _, _, ok := (Deploy{Path: "/apps"}).runInvocation("gateway"); ok {
		t.Error("a target that says nothing about starting should have no run step")
	}
}

// A container deployment is build, save, carry, load, run — in that order, with
// the image named where the spec left a placeholder.
func TestDockerPlanRendersTheSteps(t *testing.T) {
	deploy := Deploy{Host: "uno-q", User: "arduino", Path: "/home/arduino/apps",
		Docker: &Docker{File: "deploy/Dockerfile", Image: "robohub-gateway", Tag: "v1",
			Run: []string{"--net", "host", "-p", "8080:8080", "{image}"}}}

	steps, err := deploy.dockerPlan("/work/gateway", "build/gateway")
	if err != nil {
		t.Fatalf("dockerPlan: %v", err)
	}
	if len(steps) != 5 {
		t.Fatalf("expected five steps, got %d: %v", len(steps), steps)
	}
	if steps[0][0] != "docker" || steps[0][1] != "build" || steps[1][0] != "docker" || steps[1][1] != "save" {
		t.Errorf("expected build then save, got %v and %v", steps[0], steps[1])
	}
	if steps[2][0] != "rsync" || steps[3][0] != "ssh" || steps[4][0] != "ssh" {
		t.Errorf("expected carry then load then run, got %v", steps[2:])
	}
	// The image reference reaches the run arguments.
	last := steps[4]
	if last[len(last)-1] != "docker run -d --net host -p 8080:8080 robohub-gateway:v1" {
		t.Errorf("unexpected run command: %q", last[len(last)-1])
	}
}

func TestDockerPlanRefusesWithoutAnImage(t *testing.T) {
	deploy := Deploy{Host: "uno-q", Path: "/apps", Docker: &Docker{}}
	if _, err := deploy.dockerPlan(".", "app"); err == nil {
		t.Fatal("expected a docker block with no image to be refused")
	}
}

// The deploy block is read from the resolved spec: a target that carries one is
// declared, and one that carries none is not.
func TestParseDeployReadsTheBlock(t *testing.T) {
	spec := []byte(`{"name":"uno-q","deploy":{"method":"rsync","host":"uno-q.local",
		"user":"arduino","path":"/home/arduino/apps",
		"docker":{"image":"robohub-gateway","tag":"v1","run":["--net","host"]}}}`)

	deploy, err := parseDeploy(spec)
	if err != nil {
		t.Fatalf("parseDeploy: %v", err)
	}
	if !deploy.declared() {
		t.Fatal("expected the deploy block to be declared")
	}
	if deploy.Host != "uno-q.local" || deploy.User != "arduino" || deploy.Path != "/home/arduino/apps" {
		t.Errorf("deploy not read: %+v", deploy)
	}
	if deploy.Docker == nil || deploy.Docker.Image != "robohub-gateway" || len(deploy.Docker.Run) != 2 {
		t.Errorf("docker not read: %+v", deploy.Docker)
	}

	if empty, err := parseDeploy([]byte(`{"name":"pico2w"}`)); err != nil || empty.declared() {
		t.Errorf("a spec with no deploy block should declare none: %+v %v", empty, err)
	}
}

// The address is a fact about the machine, so the environment can redirect a
// deploy without editing a spec.
func TestEnvironmentOverridesTheTarget(t *testing.T) {
	t.Setenv("RH_DEPLOY_HOST", "192.168.1.50")
	t.Setenv("RH_DEPLOY_USER", "pi")
	t.Setenv("RH_DEPLOY_PORT", "2222")

	deploy := overrideFromEnv(Deploy{Host: "uno-q.local", User: "arduino", Path: "/apps"})
	if deploy.Host != "192.168.1.50" || deploy.User != "pi" || deploy.Port != 2222 {
		t.Errorf("override not applied: %+v", deploy)
	}
	if deploy.Path != "/apps" {
		t.Errorf("an unset variable should leave the spec's value: %+v", deploy)
	}
}

// A dry run prints and runs nothing: that is what makes a deployment inspectable
// before it touches a board.
func TestDryRunRunsNothing(t *testing.T) {
	original := commandRunner
	defer func() { commandRunner = original }()

	ran := false
	commandRunner = func(name string, args ...string) *exec.Cmd {
		ran = true
		return exec.Command("true")
	}

	steps := [][]string{{"rsync", "-az", "app", "uno-q:/apps/app"}}
	if err := runSteps(steps, true); err != nil {
		t.Fatalf("runSteps: %v", err)
	}
	if ran {
		t.Fatal("a dry run must not run anything")
	}
}

// The container facts are the project's, so the environment can supply them and
// a board spec does not have to carry one project's image name.
func TestEnvironmentSuppliesTheContainerFacts(t *testing.T) {
	t.Setenv("RH_DEPLOY_DOCKER_IMAGE", "robohub-gateway")
	t.Setenv("RH_DEPLOY_DOCKER_TAG", "v2")
	t.Setenv("RH_DEPLOY_DOCKER_FILE", "deploy/Dockerfile")
	t.Setenv("RH_DEPLOY_DOCKER_RUN", "--net host -p 8080:8080 {image}")

	deploy := overrideFromEnv(Deploy{Host: "uno-q", User: "arduino", Path: "/home/arduino/apps"})
	if deploy.Docker == nil {
		t.Fatal("expected the environment to supply a docker block")
	}
	if deploy.Docker.Image != "robohub-gateway" || deploy.Docker.Tag != "v2" ||
		deploy.Docker.File != "deploy/Dockerfile" {
		t.Errorf("container facts not applied: %+v", deploy.Docker)
	}
	if len(deploy.Docker.Run) != 5 || deploy.Docker.Run[4] != "{image}" {
		t.Errorf("run arguments not applied: %+v", deploy.Docker.Run)
	}

	steps, err := deploy.dockerPlan("/work/gateway", "build/robohub-gateway")
	if err != nil {
		t.Fatalf("dockerPlan: %v", err)
	}
	if steps[0][3] != "/work/gateway/deploy/Dockerfile" {
		t.Errorf("the Dockerfile should be the one named: %v", steps[0])
	}
}
