package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/rho/rho-auth/src/auth"
)

// readPasswordFile is the password a command was handed in a file — never on
// its command line, where every process on the machine can read it. The file is
// the caller's to remove.
func readPasswordFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("--password-file names the file the password is in")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

// user implements `rho auth user add|verify|passwd|set|remove|list`.
//
//	rho auth user add    --store F --name ana --role operator [--display Ana] [--grants mesh:*] --password-file P
//	rho auth user verify --store F --name ana --password-file P     prints name, display, role, grants
//	rho auth user passwd --store F --name ana --password-file P
//	rho auth user set    --store F --name ana [--display D] [--role R] [--grants G|-]
//	rho auth user remove --store F --name ana
//	rho auth user list   --store F                                   name, display, role, grants, created
func user(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: rho auth user <add|verify|passwd|set|remove|list> --store FILE [--name NAME] ...")
		return 2
	}
	options := authOptions(args[1:])
	path := options["store"]
	if path == "" {
		fmt.Fprintln(os.Stderr, "rho auth user: --store names the users file")
		return 2
	}
	name := options["name"]
	fail := func(err error) int {
		fmt.Fprintf(os.Stderr, "rho auth user %s: %v\n", args[0], err)
		return 1
	}
	switch args[0] {
	case "add":
		password, err := readPasswordFile(options["password-file"])
		if err != nil {
			return fail(err)
		}
		if err := auth.AddUser(path, auth.User{Name: name, Display: options["display"], Role: options["role"],
			Grants: splitOption(options["grants"])}, password); err != nil {
			return fail(err)
		}
		log.Info("user added", "name", name, "role", options["role"])
		return 0
	case "verify":
		password, err := readPasswordFile(options["password-file"])
		if err != nil {
			return fail(err)
		}
		found, err := auth.VerifyUser(path, name, password)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", found.Name, found.Display, found.Role, strings.Join(found.Grants, ","))
		return 0
	case "passwd":
		password, err := readPasswordFile(options["password-file"])
		if err != nil {
			return fail(err)
		}
		if err := auth.SetPassword(path, name, password); err != nil {
			return fail(err)
		}
		log.Info("password changed", "name", name)
		return 0
	case "set":
		if err := auth.UpdateUser(path, name, options["display"], options["role"], options["grants"]); err != nil {
			return fail(err)
		}
		return 0
	case "remove":
		if err := auth.RemoveUser(path, name); err != nil {
			return fail(err)
		}
		log.Info("user removed", "name", name)
		return 0
	case "list":
		users, err := auth.LoadUsers(path)
		if err != nil {
			return fail(err)
		}
		names := make([]string, 0, len(users))
		for one := range users {
			names = append(names, one)
		}
		sort.Strings(names)
		for _, one := range names {
			u := users[one]
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n", u.Name, u.Display, u.Role, strings.Join(u.Grants, ","), u.Created)
		}
		return 0
	}
	fmt.Fprintf(os.Stderr, "rho auth user: unknown command %q\n", args[0])
	return 2
}
