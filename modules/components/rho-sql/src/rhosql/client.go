//
// @file client.go
// @brief Talking to an engine.
//
// A caller holds a client, not a process. The client finds the engine plugin, asks
// it what it is, and from then on sends statements to it over a session that
// stays open for as long as the caller does. Everything a service does with a
// database goes through here, and nothing a service does with a database knows
// which engine answered.
//
// A client is safe for concurrent use and serialises its own requests, because
// a single engine session is a single conversation: two requests written at once
// would be two answers for one.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhosql

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"rho/src/deps"
	"rho/src/plugins"
)

// Client talks to one engine plugin.
type Client struct {
	plugin  plugins.Plugin
	session *plugins.Session
	engine  Engine
	// environment is added to the engine process, and is how a caller points it
	// at a database of its own. It is set before the session starts, because a
	// process reads its environment once.
	environment []string

	mu sync.Mutex
	// described is whether the engine has been asked what it is. A caller that
	// sends a statement without asking has not checked that the engine speaks
	// the marker the statement is written in, so the client asks first, once.
	described bool
}

// Open finds an engine plugin and prepares to talk to it.
//
// The engine is not started here. A service that is configured with a database
// it never touches pays nothing for it, and the process begins with the first
// statement.
//
// A name is required. There is no default engine: which database a service runs
// against is a deployment decision, and a toolchain that guessed one would
// send a service's rows somewhere nobody chose.
func Open(name string) (*Client, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf(
			"no engine named; a service says which database it runs against, " +
				"as an engine plugin name such as rho-sqlite")
	}
	home := deps.RhoHome()
	if home == "" {
		return nil, fmt.Errorf(
			"no Rho installation was found; a component is used from the local " +
				"registry, which 'rho install' creates")
	}
	// The installed plugins, and not the tree: an engine is a program, and one
	// that arrived with a repository is one that came from whoever wrote the
	// repository. The source tree is trusted nowhere.
	source := plugins.Source{RhoHome: home}
	for _, plugin := range source.All() {
		if plugin.Name != name {
			continue
		}
		return &Client{plugin: plugin}, nil
	}
	return nil, fmt.Errorf(
		"no engine plugin named %q is installed; an engine is a plugin, "+
			"published into the local registry like any other", name)
}

// Plugin is the plugin this client talks to, which is what a caller reports when
// it wants to say which engine answered.
func (c *Client) Plugin() plugins.Plugin { return c.plugin }

// UseEnvironment tells the engine something about where to run, which is how a
// caller asks it to work against a database of its own rather than the one a
// deployment configured.
//
// It has to be called before the first request, because an engine reads its
// environment once when it starts.
func (c *Client) UseEnvironment(values ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		return fmt.Errorf("the engine has already started, so it has read its environment")
	}
	c.environment = append(c.environment, values...)
	return nil
}

// OpenAt is Open with somewhere for the engine to run.
//
// It is what a component uses to generate a description: describing a statement
// needs the statement prepared, preparing it needs the tables, and creating them
// in the database a service runs against would be a side effect of asking a
// question.
func OpenAt(name string, environment ...string) (*Client, error) {
	client, err := Open(name)
	if err != nil {
		return nil, err
	}
	if err := client.UseEnvironment(environment...); err != nil {
		return nil, err
	}
	return client, nil
}

// Engine is what the engine said about itself, once it has been asked. A caller
// that reads it before the engine has answered is told the caller has to ask,
// because an Engine with no markers says a caller can trust nothing.
func (c *Client) Engine() (Engine, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.described {
		return Engine{}, fmt.Errorf("the engine has not been asked what it is yet")
	}
	return c.engine, nil
}

// Describe asks the engine what it is.
func (c *Client) Describe() (Engine, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.describeLocked()
}

// describeLocked is Describe for a caller already holding the lock.
func (c *Client) describeLocked() (Engine, error) {
	if c.described {
		return c.engine, nil
	}
	if c.session == nil {
		if err := c.openSession(); err != nil {
			return Engine{}, err
		}
	}
	answer, err := c.session.Do(ComponentName, HookDescribe, c.plugin.Name, DescribeRequest{})
	if err != nil {
		return Engine{}, err
	}
	// An engine answers `describe` wrapped, so that a future hook can be added
	// to the envelope without changing what every existing engine answers.
	described, err := decodePayload[DescribeResponse](answer, HookDescribe)
	if err != nil {
		return Engine{}, err
	}
	engine := described.Engine
	if engine.Name == "" {
		// An engine that does not say what it is cannot be written about in a
		// project, so the refusal is here rather than at the first statement.
		return Engine{}, fmt.Errorf("engine %s did not say what it is", c.plugin.Name)
	}
	c.engine = engine
	c.described = true
	return engine, nil
}

// openSession starts the engine, once. The caller holds the lock.
func (c *Client) openSession() error {
	if c.session != nil {
		return nil
	}
	session, err := plugins.Open(plugins.SessionOptions{
		Plugin:      c.plugin,
		Environment: c.environment,
	})
	if err != nil {
		return err
	}
	c.session = session
	return nil
}

// Migrate creates the tables a schema declares.
func (c *Client) Migrate(schema Schema, force bool) (MigrateResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.describeLocked(); err != nil {
		return MigrateResponse{}, err
	}
	answer, err := c.session.Do(ComponentName, HookMigrate, c.plugin.Name, MigrateRequest{
		Schema: schema, Force: force,
	})
	if err != nil {
		return MigrateResponse{}, err
	}
	return decodePayload[MigrateResponse](answer, HookMigrate)
}

// Query runs one named statement.
func (c *Client) Query(request QueryRequest) (QueryResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.describeLocked(); err != nil {
		return QueryResponse{}, err
	}
	answer, err := c.session.Do(ComponentName, HookQuery, c.plugin.Name, request)
	if err != nil {
		return QueryResponse{}, err
	}
	return decodePayload[QueryResponse](answer, HookQuery)
}

// DescribeQuery asks what a statement returns, without running it.
func (c *Client) DescribeQuery(request DescribeQueryRequest) (DescribeQueryResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.describeLocked(); err != nil {
		return DescribeQueryResponse{}, err
	}
	answer, err := c.session.Do(ComponentName, HookDescribeQuery, c.plugin.Name, request)
	if err != nil {
		return DescribeQueryResponse{}, err
	}
	return decodePayload[DescribeQueryResponse](answer, HookDescribeQuery)
}

// Close stops the engine, and is safe to call more than once.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil {
		return nil
	}
	err := c.session.Close()
	c.session = nil
	c.described = false
	return err
}

// decodePayload reads a hook's answer.
//
// A plugin that answered with something other than the shape the hook asks for
// is a plugin that is not this version of the protocol, and saying so is more
// useful than an empty answer a caller would read as "no rows".
//
// What a plugin could not do is reported in its diagnostics rather than in its
// payload, because a failed hook has no payload. That is read first: an engine
// that refused and said why is more useful than one reported as having said
// nothing, and the reason is the whole of what a caller can act on.
func decodePayload[T any](answer plugins.Response, hook string) (T, error) {
	var out T
	for _, item := range answer.Diagnostics {
		if item.Level == "error" {
			return out, EngineError{Response: ErrorResponse{
				Kind:    ErrorEngine,
				Message: item.Message,
			}}
		}
	}
	if len(answer.Payload) == 0 {
		return out, fmt.Errorf("the engine answered %s with nothing", hook)
	}
	if err := json.Unmarshal(answer.Payload, &out); err != nil {
		return out, fmt.Errorf(
			"the engine's answer to %s is not the shape this component expects: %w", hook, err)
	}
	return out, nil
}
