//
// @file emit_go_sqlstore.go
// @brief The registry's records, kept in tables rather than in files.
//
// A registry answers one question and, in a table, what the answer is made of:
// who an identity is, what it may do, and which modules it published. This is
// the code that keeps them there.
//
// The tables are the ones the project declared in its schema file, and this
// creates them from the schema it was given rather than from statements written
// here. That is the point of declaring them: the database a service creates and
// the code generated from the same declarations cannot come to describe two
// different things, and a service that adds a column declares it once.
//
// What stays on disk is what is derived rather than recorded: the source tree a
// package is unpacked into and the documentation generated from it. Those are
// many small files, they are rebuilt from the package, and a database holding
// them would be a cache. The package itself is a row, because an upload and the
// record of who made it are one commit — a registry that wrote a file and then a
// row can be interrupted between them, and what is left is a module nobody owns.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"strings"
)

// sqlStoreSource is the registry's table-backed store, written into a generated
// project beside the rest of the store.
//
// The schema is a placeholder because it is the project's: a store that created
// its own tables would create the tables this file says rather than the ones the
// project declared, and the two would drift the first time a column was added.
const sqlStoreSource = `
// The databases one process has open, by the file each is in.
//
// A store is built per call, because a handler reaches it as a function, so a
// connection held on the store would be opened and closed per statement. The
// pools are held here instead and opened once each: a registry serving a browser
// should not open a database for every route it answers.
//
// They are keyed by the file rather than held as one, because the file is a
// deployment decision and a process may be asked to serve more than one — a test
// running against a private data directory is the ordinary case, and a single
// pool would hand it the first directory it saw.
var (
	registryDBMu    sync.Mutex
	registryDBByDir = map[string]*sql.DB{}
)

// openRegistryDB returns the connection for the configured database, creating
// the tables the project declared the first time that file is opened.
//
// The schema is applied here rather than by the service, because a service that
// had to remember to create its own tables would have a first request that
// failed rather than a first request that created them.
func openRegistryDB() (*sql.DB, error) {
	path := registryDatabase()
	registryDBMu.Lock()
	defer registryDBMu.Unlock()
	if existing, held := registryDBByDir[path]; held {
		return existing, nil
	}
	if directory := filepath.Dir(path); directory != "" {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return nil, fmt.Errorf("the registry's database directory cannot be created: %%w", err)
		}
	}
	handle, err := sql.Open(registryDriverName(), path)
	if err != nil {
		return nil, fmt.Errorf("the registry's database cannot be opened: %%w", err)
	}
	if err := handle.Ping(); err != nil {
		handle.Close()
		return nil, fmt.Errorf("the registry's database cannot be reached: %%w", err)
	}
	if err := registryApplySchema(handle); err != nil {
		handle.Close()
		return nil, err
	}
	registryDBByDir[path] = handle
	return handle, nil
}

// registryDriverName is the driver the records are kept through.
//
// It is the same name the generated statements run under, and it is read from
// the environment rather than compiled in: a database is a deployment decision,
// and a driver compiled into the binary would be one this build made for it.
// The sql port names it too, when a project asked for both, and this is the same
// name, so a statement and the record it belongs to go to one place.
func registryDriverName() string {
	if name := strings.TrimSpace(os.Getenv("RHO_SQL_DRIVER")); name != "" {
		return name
	}
	return "sqlite"
}

// registryDatabase is the database the records are kept in.
//
// It is named by the environment rather than by the data directory, because a
// database is a deployment decision the same way a driver is: the same binary
// serves a development database and a production one.
func registryDatabase() string {
	if path := strings.TrimSpace(os.Getenv("RHO_SQL_DATABASE")); path != "" {
		return path
	}
	return filepath.Join(newFSStore().root, "registry.db")
}

// registryApplySchema creates the tables a project declared.
//
// A table that is already there is what a service restarted against its own
// database looks like, and the second application changes nothing.
func registryApplySchema(db *sql.DB) error {
	for _, statement := range registryStatements(registrySchemaText) {
		if _, err := db.Exec(statement); err != nil &&
			!strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return fmt.Errorf("the registry's tables cannot be created: %%w", err)
		}
	}
	return nil
}

// registryStatements splits a schema into the statements an engine takes.
//
// A semicolon inside a string literal is not the end of a statement, and a
// default of § + "datetime('now')" + § has one.
func registryStatements(schema string) []string {
	out := []string{}
	current := strings.Builder{}
	inString := false
	for i := 0; i < len(schema); i++ {
		c := schema[i]
		switch {
		case c == '\'':
			inString = !inString
			current.WriteByte(c)
		case c == ';' && !inString:
			if statement := strings.TrimSpace(current.String()); statement != "" {
				out = append(out, statement)
			}
			current.Reset()
		default:
			current.WriteByte(c)
		}
	}
	if statement := strings.TrimSpace(current.String()); statement != "" {
		out = append(out, statement)
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  Identities                                                                  */
/*                                                                              */
/*  A registry has no registration: an identity is named by whatever credential  */
/*  presented it, and exists once it has published something.                     */
/* -------------------------------------------------------------------------- */

// registryUserID records an identity, or answers the row it already had.
func registryUserID(db *sql.DB, name string) (int64, error) {
	if strings.TrimSpace(name) == "" {
		return 0, fmt.Errorf("an identity must be named")
	}
	if _, err := db.Exec(§INSERT INTO users (name) VALUES (?) ON CONFLICT (name) DO NOTHING§, name); err != nil {
		return 0, err
	}
	var id int64
	if err := db.QueryRow(§SELECT id FROM users WHERE name = ?§, name).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// registryRoleID is a role by the name the schema gave it, or nothing.
//
// A role nobody has used is a row this has not asked for yet, so a role that is
// not there is not an error: the caller records the role out as a name.
func registryRoleID(db *sql.DB, name string) (int64, bool) {
	var id int64
	if err := db.QueryRow(§SELECT id FROM roles WHERE name = ?§, name).Scan(&id); err != nil {
		return 0, false
	}
	return id, true
}

/* -------------------------------------------------------------------------- */
/*  Modules                                                                     */
/* -------------------------------------------------------------------------- */

// registryModuleID is the row a module name is recorded in, or nothing.
func registryModuleID(db *sql.DB, name string) (int64, bool, error) {
	var id int64
	err := db.QueryRow(§SELECT id FROM modules WHERE name = ?§, name).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}
	return 0, false, nil
}

// registryClaimModule records who published a name, and how visible it is.
//
// The owner is written only when the name was free, which is what makes a name
// once-claimed: a re-publish by a different identity does not take it over.
func registryClaimModule(db *sql.DB, name, owner, maintainer, visibility string) error {
	ownerID, err := registryUserID(db, owner)
	if err != nil {
		return err
	}
	if visibility == "" {
		visibility = VisibilityPublic
	}
	if _, err := db.Exec(
		§INSERT INTO modules (name, owner_id, visibility) VALUES (?, ?, ?) ON CONFLICT (name) DO NOTHING§,
		name, ownerID, visibility); err != nil {
		return err
	}
	moduleID, held, err := registryModuleID(db, name)
	if err != nil {
		return err
	}
	if !held {
		return fmt.Errorf("%%s could not be claimed", name)
	}
	// The owner maintains what they own and the identity the manifest names as
	// maintainer maintains it too. Both are recorded, because letting a second
	// identity in later has to be a change to what is stored rather than a
	// change to what is read.
	if err := registryAddMaintainer(db, moduleID, owner, RoleOwner); err != nil {
		return err
	}
	if maintainer != "" && maintainer != owner {
		return registryAddMaintainer(db, moduleID, maintainer, RoleMaintainer)
	}
	return nil
}

// registryAddMaintainer records that an identity may publish to a module.
func registryAddMaintainer(db *sql.DB, moduleID int64, name, role string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	userID, err := registryUserID(db, name)
	if err != nil {
		return err
	}
	var roleID any
	if id, found := registryRoleID(db, role); found {
		roleID = id
	}
	// The pair is recorded once. A table constraint would say that once for
	// good, but the declarations state which columns make a maintainer and this
	// reads them from the same place, so the check is here rather than being
	// assumed of a table whose shape this does not own.
	var already int
	row := db.QueryRow(
		§SELECT 1 FROM module_maintainers WHERE module_id = ? AND user_id = ?§,
		moduleID, userID)
	if row.Scan(&already) == nil {
		_, err = db.Exec(§UPDATE module_maintainers SET role_id = ? WHERE module_id = ? AND user_id = ?§,
			roleID, moduleID, userID)
		return err
	}
	_, err = db.Exec(
		§INSERT INTO module_maintainers (module_id, user_id, role_id) VALUES (?, ?, ?)§,
		moduleID, userID, roleID)
	return err
}

// registryHeld is who holds a module, as a table records it.
type registryHeld struct {
	Name           string
	Owner          string
	Maintainers    []string
	Visibility     string
	FirstPublished time.Time
}

// IsPrivate reports whether a held module is its owner's alone.
func (h *registryHeld) IsPrivate() bool {
	return h != nil && h.Visibility == VisibilityPrivate
}

// MayPublish reports whether an identity may add a version to this module.
//
// It is read from the maintainers rather than compared to the owner, so letting
// somebody in is a change to a row and not a change to this file. An identity
// with no row cannot publish, and that is what keeps a re-publish from taking a
// module over.
func (h *registryHeld) MayPublish(identity string) bool {
	if h == nil || identity == "" {
		return false
	}
	if h.Owner == identity {
		return true
	}
	for _, maintainer := range h.Maintainers {
		if maintainer == identity {
			return true
		}
	}
	return false
}

// holders is who may publish, as a message names them.
func (h *registryHeld) holders() string {
	if h == nil {
		return "its publishers"
	}
	if len(h.Maintainers) == 0 {
		return h.Owner
	}
	return strings.Join(h.Maintainers, ", ")
}

// registryReadPublisher is who may withdraw a module, or nil when the module is
// not held.
func registryReadPublisher(db *sql.DB, name string) (*registryHeld, error) {
	var id int64
	var visibility string
	var published string
	var owner sql.NullString
	err := db.QueryRow(
		§SELECT m.id, m.visibility, m.created_at, u.name
		 FROM modules m LEFT JOIN users u ON u.id = m.owner_id
		 WHERE m.name = ?§, name).Scan(&id, &visibility, &published, &owner)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	held := &registryHeld{
		Name:           name,
		Owner:          owner.String,
		Visibility:     visibility,
		FirstPublished: registryTime(published),
	}
	rows, err := db.Query(
		§SELECT u.name FROM module_maintainers mm
		 JOIN users u ON u.id = mm.user_id
		 WHERE mm.module_id = ? ORDER BY u.name§, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var maintainer string
		if err := rows.Scan(&maintainer); err != nil {
			return nil, err
		}
		held.Maintainers = append(held.Maintainers, maintainer)
	}
	// A name with no maintainer row is held by its owner alone, which is the
	// state a claim leaves before anybody else is let in.
	if len(held.Maintainers) == 0 && held.Owner != "" {
		held.Maintainers = []string{held.Owner}
	}
	return held, nil
}

// registryTime is a moment a column recorded as text, or the zero time.
//
// The declarations say a created-at column is text, because a moment a database
// generates is a value it writes and a value it is asked to compare; how it is
// stored is the engine's business. It arrives as text here, and is read as one.
func registryTime(value string) time.Time {
	for _, layout := range []string{
		time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05",
	} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

// registrySetVisibility records whether a module is anybody's to install or its
// owner's alone.
func registrySetVisibility(db *sql.DB, name, visibility string) error {
	if visibility != VisibilityPrivate {
		visibility = VisibilityPublic
	}
	result, err := db.Exec(§UPDATE modules SET visibility = ? WHERE name = ?§, visibility, name)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return fmt.Errorf("%%s is not published here", name)
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Versions                                                                    */
/* -------------------------------------------------------------------------- */

// registryRecordVersion writes a published version: the record of it, who made
// it, and the package itself, in one commit.
//
// The package is a column because a publish is one fact. A registry that wrote
// the package and then the record of who published it can be interrupted between
// the two, and what is left is a version nobody can withdraw.
func registryRecordVersion(db *sql.DB, name, version, owner string, payload []byte, meta moduleMeta) error {
	moduleID, held, err := registryModuleID(db, name)
	if err != nil {
		return err
	}
	if !held {
		return fmt.Errorf("%%s is not published here", name)
	}
	ownerID, err := registryUserID(db, owner)
	if err != nil {
		return err
	}
	encoded, err := jsonenc.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		§INSERT INTO module_versions (module_id, version, payload, meta, publisher_id)
		 VALUES (?, ?, ?, ?, ?)§, moduleID, version, payload, string(encoded), ownerID)
	return err
}

// registryHasVersion reports whether a module already holds a version, which is
// what refuses a re-publish.
func registryHasVersion(db *sql.DB, name, version string) bool {
	row := db.QueryRow(
		§SELECT 1 FROM module_versions mv JOIN modules m ON m.id = mv.module_id
		 WHERE m.name = ? AND mv.version = ?§, name, version)
	var found int
	return row.Scan(&found) == nil
}

// registryPayload is the published package, whole.
func registryPayload(db *sql.DB, name, version string) ([]byte, error) {
	var payload []byte
	err := db.QueryRow(
		§SELECT mv.payload FROM module_versions mv JOIN modules m ON m.id = mv.module_id
		 WHERE m.name = ? AND mv.version = ?§, name, version).Scan(&payload)
	return payload, err
}

// registryMeta is the manifest as it was published, and when.
func registryMeta(db *sql.DB, name, version string) (moduleMeta, bool) {
	var encoded, publishedAt string
	err := db.QueryRow(
		§SELECT mv.meta, mv.published_at FROM module_versions mv JOIN modules m ON m.id = mv.module_id
		 WHERE m.name = ? AND mv.version = ?§, name, version).Scan(&encoded, &publishedAt)
	if err != nil {
		return moduleMeta{}, false
	}
	var meta moduleMeta
	if err := jsonenc.Unmarshal([]byte(encoded), &meta); err != nil {
		return moduleMeta{}, false
	}
	// The time is a column rather than a field of the manifest, so it is put on
	// the value here, where both the query and the reader are.
	meta.PublishedAt = publishedAt
	return meta, true
}

// registryVersions lists a module's versions, oldest first, which is the order
// they were published in.
func registryVersions(db *sql.DB, name string) []string {
	rows, err := db.Query(
		§SELECT mv.version FROM module_versions mv JOIN modules m ON m.id = mv.module_id
		 WHERE m.name = ? ORDER BY mv.published_at, mv.id§, name)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return out
		}
		out = append(out, version)
	}
	return out
}

// registryDeleteVersion withdraws one version of a module.
func registryDeleteVersion(db *sql.DB, name, version string) error {
	_, err := db.Exec(
		§DELETE FROM module_versions
		 WHERE module_id = (SELECT id FROM modules WHERE name = ?) AND version = ?§, name, version)
	return err
}

// registryDeleteModule withdraws a module name, once its last version has gone.
//
// The maintainers go with it, because a name nobody holds is a name nobody has a
// relationship to, and a name that is free is one another identity may claim.
func registryDeleteModule(db *sql.DB, name string) error {
	_, err := db.Exec(§DELETE FROM modules WHERE name = ?§, name)
	return err
}

// registryModuleNames is every module the registry holds, in the order a
// listing shows them.
func registryModuleNames(db *sql.DB) []string {
	rows, err := db.Query(§SELECT name FROM modules ORDER BY name§)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return out
		}
		out = append(out, name)
	}
	return out
}

// registryRecordToken records a credential the service issued.
//
// The token is signed and checks out on its own, so this row is what the service
// can revoke and what it can show when somebody asks what has been issued.
func registryRecordToken(db *sql.DB, subject, registry, scopes, expires string) error {
	userID, err := registryUserID(db, subject)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		§INSERT INTO tokens (user_id, registry, scopes, expires_at) VALUES (?, ?, ?, ?)§,
		userID, registry, scopes, expires)
	return err
}
`

// sqlStore renders the table-backed store for a project that declared its
// tables.
//
// The schema is embedded rather than fetched: the tables are created from the
// same text the project's generated statements were built from, so what this
// creates and what the code assumes cannot be two different databases.
func sqlStore(ctx PortContext) string {
	schema := strings.TrimSpace(ctx.SQLSchema)
	if schema == "" {
		schema = "-- This project declared no schema.\n"
	}
	source := strings.ReplaceAll(sqlStoreSource, "§", "`")
	return strings.ReplaceAll(source, "registrySchemaText", goQuoted(schema))
}
