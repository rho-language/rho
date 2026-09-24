package registry

import "testing"

/* -------------------------------------------------------------------------- */
/*  Test: ListRemoteModules                                                    */
/* -------------------------------------------------------------------------- */

func TestListRemoteModules(t *testing.T) {
    modules := ListRemoteModules()
    if len(modules) == 0 {
        t.Fatalf("expected simulated registry entries")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: RemoteVersions                                                       */
/* -------------------------------------------------------------------------- */

func TestRemoteVersions(t *testing.T) {
    versions := RemoteVersions("math")
    if len(versions) == 0 {
        t.Fatalf("expected at least one version for math")
    }
}
