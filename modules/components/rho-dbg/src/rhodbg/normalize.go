//
// @file normalize.go
// @brief A part number, and the names a vendor publishes it under.
//
// A board's spec says `STM32H753ZI`: a part, a package, a temperature grade and a
// revision, in one string. A vendor's register map is published per *device* under
// the device's own name — `STM32H753x`, one file for the H742, H743, H750 and H753
// at once, with no package code in it at all. So getting from one to the other is a
// question of spelling, and it is answered by trying the possibilities in order from
// the most specific to the least and taking the first the vendor publishes.
//
// Which is only trustworthy because of what happens when none of them matches and
// what happens when two do: nothing is fetched, and both are named. A register map
// for a neighbouring part is a register tree full of plausible values belonging to
// a different chip, and the person reading it has no way to tell.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodbg

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// NormalizeTarget is the names a part number might be published under, most
// specific first.
//
// The order is the whole design. A candidate that is *less* specific is only reached
// when every candidate before it published nothing, so a part number that matches
// exactly is never satisfied by its family's file, and a file for a line is never
// preferred over the file for the part on it.
//
// The rules are the ones vendors' naming actually follows:
//
//   - the part as written, since a vendor that publishes `STM32F103` for the F103
//     publishes it under that name
//   - without hyphens and underscores, because a vendor publishes `nRF52840` and a
//     part number is written `nRF52840-QIAA`
//   - without its package and temperature code — `STM32H753ZIT6` and `STM32H753ZI`
//     are one device, and the letters and digits after the device say how it is
//     packaged and how hot it is
//   - with the wildcard a vendor writes for a line — `STM32H753` is published as
//     `STM32H753x`, one file for four parts
//   - with the family wildcard — `STM32H7` covers H74x, H75x, H7A3x and H7B3x, so
//     `STM32H7xx` is the last resort and the first thing a person should be told is
//     not right, because it stands for a line rather than for a part
func NormalizeTarget(raw string) []string {
	target := strings.ToLower(strings.TrimSpace(raw))
	if target == "" {
		return nil
	}

	candidates := []string{target}
	seen := map[string]bool{target: true}
	add := func(candidate string) {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" || seen[candidate] {
			return
		}
		seen[candidate] = true
		candidates = append(candidates, candidate)
	}

	// A separator says where the device ends, which is worth more than any amount of
	// stripping: `nRF52840-QIAA` is the device `nRF52840` and the package `QIAA`, and
	// the hyphen is the only thing in the string that says so.
	if separator := strings.LastIndexAny(target, "-_"); separator > 0 {
		add(target[:separator])
	}
	// And with the separators gone, the same string in the form a vendor publishes.
	add(strings.NewReplacer("-", "", "_", "").Replace(target))

	// The package and temperature code, taken off the device as written and as
	// separated — in steps, because there is usually more than one: `STM32H743ZIT6`
	// is the device `STM32H743` behind a package `ZI` and a temperature grade `T6`,
	// and a vendor publishes the device alone.
	for _, current := range []string{candidates[0], candidates[len(candidates)-1]} {
		for _, device := range stripPackageCodes(current) {
			add(device)
		}
	}

	// The line a part belongs to, and the family it belongs to — from the device
	// rather than from the string as written, because a package code is not part of
	// the device and a family worked out from one is a different family.
	//
	// `STM32H753` is on the `H753x` line and in the `STM32H7` family, and a vendor
	// writes both: the first as the device, the second as `STM32H7xx`.
	for _, current := range []string{candidates[0], candidates[len(candidates)-1]} {
		if prefix, digits := splitFamily(current); prefix != "" {
			// The line: the vendor's own digits and the wildcard it writes them with.
			add(prefix + digits + "x")
			// The family: the first digit of the line, and the two wildcards, which
			// stand for a line rather than for a part and are the last thing to try.
			if len(digits) > 0 {
				add(prefix + digits[:1] + "xx")
			}
		}
	}
	return candidates
}

// A digitRun is a run of digits anywhere in a part number, and the longest one is
// the device's.
//
// The longest, not the last: `STM32H743ZIT6` has `743` in it and a single `6` for
// the temperature grade, and the grade is not the device. A device's number is
// longer than any code that follows it, which is true on every vendor whose naming
// this rule fits, and it is what makes the cut possible without a table of
// vendors.
var digitRun = regexp.MustCompile(`[0-9]+`)

// stripPackageCodes is the names a part number could be published as, from most to
// least complete.
//
// The rule is one: a device ends where its vendor's digits end. `STM32H743ZIT6` is
// the device `STM32H743` behind a package `ZIT6`, and the same rule gives
// `nRF52840-QIAA` the device `nRF52840` and `STM32F446` itself.
//
// Only the device is produced. The variant that keeps the package — `STM32H753ZI` —
// is not offered, because no vendor publishes a device under a name that carries its
// packaging, and a candidate nobody publishes costs a lookup and can only mislead.
func stripPackageCodes(raw string) []string {
	target := strings.ToLower(strings.TrimSpace(raw))
	widest, end := 0, -1
	for _, run := range digitRun.FindAllStringIndex(target, -1) {
		if width := run[1] - run[0]; width > widest {
			widest, end = width, run[1]
		}
	}
	if end < 0 {
		return nil
	}
	// A cut before the vendor's family letter is not a device: it is a word.
	if end < 2 {
		return nil
	}
	return []string{target[:end]}
}

// family matches a part number's vendor prefix and the digits that follow it:
// `stm32h` and `753` for `stm32h753`, which is ST's convention — the family's own
// letter comes after the vendor's, which is what makes `STM32H7` derivable from a
// part number at all.
var family = regexp.MustCompile(`^([a-z]+[0-9]*[a-z])([0-9]+[a-z]?)$`)

// splitFamily is a part number's vendor prefix and the digits after it.
func splitFamily(target string) (string, string) {
	match := family.FindStringSubmatch(target)
	if match == nil {
		return "", ""
	}
	return match[1], match[2]
}

/* -------------------------------------------------------------------------- */
/*  Matching                                                                   */
/* -------------------------------------------------------------------------- */

// FindDevice is the register map a part number means, and whether the naming
// settles it.
//
// Two outcomes are refusals rather than answers, and both are about the same thing:
// a register map is only worth showing when it is the map of *this* part. A name no
// file matches is a part the vendor has not published one for; a name two files
// match is a name two vendors use, which no amount of spelling decides. Both are
// reported with what was found, because the next step differs: one is "this part has
// no map" and the other is "say which vendor you mean".
func FindDevice(index Index, part string) (SVDSource, []string, error) {
	candidates := NormalizeTarget(part)
	if len(candidates) == 0 {
		return SVDSource{}, nil, errNoPart
	}
	tried := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		tried = append(tried, candidate)
		paths, published := index.published(candidate)
		if !published {
			continue
		}
		if len(paths) > 1 {
			// Two vendors publish a device of this name. Choosing between them would
			// be a guess about which chip is on the desk, and the cost of guessing
			// wrong here is a peripheral tree full of another chip's registers.
			return SVDSource{}, tried, &ambiguousError{
				device: candidate,
				paths:  append([]string{}, paths...),
			}
		}
		return sourceOfPath(paths[0]), tried, nil
	}
	return SVDSource{}, tried, &unknownDeviceError{part: part, tried: tried}
}

// sourceOfPath is a path in the repository as the source that names it: the file
// without the directory the repository keeps every vendor in.
func sourceOfPath(p string) SVDSource {
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return SVDSource{Path: path.Base(p), Device: strings.TrimSuffix(path.Base(p), ".svd") + ".svd"}
	}
	device := path.Base(p)
	return SVDSource{
		Path:   path.Join(parts[len(parts)-2], device),
		Vendor: parts[len(parts)-2],
		Device: device,
	}
}

// errNoPart is a session that named no part at all, which is a host rather than a
// board and has no peripherals to describe.
var errNoPart = &unknownDeviceError{part: ""}

// IsUnknownDevice reports whether a failure is a part the vendor has not published
// a register map for, or a name more than one vendor uses.
func IsUnknownDevice(err error) bool {
	var unknown *unknownDeviceError
	if errors.As(err, &unknown) {
		return true
	}
	var ambiguous *ambiguousError
	return errors.As(err, &ambiguous)
}

// unknownDeviceError is a part number nothing published matches, with the spellings
// it was looked under — because a person reading the failure needs to know whether
// their part number is the problem or whether the vendor simply has nothing.
type unknownDeviceError struct {
	part  string
	tried []string
}

func (e *unknownDeviceError) Error() string {
	if e.part == "" {
		return "no part is named, so there is no register map to look for"
	}
	return fmt.Sprintf("the published set has no register map for %s, under any of the names it is written as", e.part)
}

// ambiguousError is a device name more than one vendor publishes.
type ambiguousError struct {
	device string
	paths  []string
}

func (e *ambiguousError) Error() string {
	return fmt.Sprintf("%s publishes a register map named %s, so which chip this is cannot be decided from the name: %s",
		e.paths[0], e.device, strings.Join(e.paths, ", "))
}
