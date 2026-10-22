# syntax=docker/dockerfile:1
#
# What CI runs, in a container, so that "works on my machine" cannot happen.
#
# The environment this reproduces is the one that matters: no `~/.rho` at all, no
# registry, no PATH entry, nothing from a previous run. That is what a GitHub
# Actions runner starts as, and it is the state in which two separate procedures
# failed — once because nothing installed the components before the tests needed
# them, and once because `rho install --update` reported success on a machine it
# had not installed anything into.
#
# So the whole procedure runs here from a bare checkout, and the exit status is the
# one CI reports. Build it and run it:
#
#   docker build -f ci/check.Dockerfile -t rho-check .
#   docker run --rm rho-check
#
# Deliberately the same entry point as the workflow: both run ./scripts/check.sh. There is
# one definition of what a check does, and this is a second way of running it
# rather than a second thing to keep in step.
FROM golang:1.26.10-bookworm

# git, because a component is published from its manifest and some of them are
# fetched from a repository; cmake and a C compiler, because the core tests build
# programs.
RUN apt-get update \
 && apt-get install -y --no-install-recommends git cmake gcc make ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY . .

# Nothing about the toolchain is set up here on purpose. No PATH entry, no tool
# home, no registry: scripts/check.sh has to produce all of it, and anything set up now is
# something the check would no longer be proving.

RUN ./scripts/check.sh

# The report of what it did, so a failure here says which module failed rather
# than only that the last line was not "Done".
CMD ["sh", "-c", "echo 'the toolchain installed by this image:' && ls ~/.rho/bin"]
