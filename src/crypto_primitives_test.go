package rho

import "testing"

// HMAC-SHA256 agrees with RFC 4231 on both backends, including a key longer than a
// block; random bytes come back as many as were asked for, and differ.
func TestCryptoPrimitivesOnBothBackends(t *testing.T) {
	c, g := runBothBackends(t, `module app

extern fn rho_hmac_sha256_hex(key_hex: string, message_hex: string) string
extern fn rho_random_hex(bytes: u32) string

fn main() void {
    print(rho_hmac_sha256_hex("0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b", "4869205468657265"))
    print("|")
    print(rho_hmac_sha256_hex("4a656665", "7768617420646f2079612077616e7420666f72206e6f7468696e673f"))
    print("|")
    print(rho_hmac_sha256_hex("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "54657374205573696e67204c6172676572205468616e20426c6f636b2d53697a65204b6579202d2048617368204b6579204669727374"))
    print("|")
    print(rho_hmac_sha256_hex("zz", "00"))
    print("|")
    let a: string = rho_random_hex(16)
    let b: string = rho_random_hex(16)
    print(int_to_str(u32(strlen(a))))
    if a != b {
        print("differ")
    }
}
`)
	want := "b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7|" +
		"5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843|" +
		"60e431591ee0b67f0d8a26aacbf5b77f8e0bc6213728c5140546040f0ee37f54|" +
		"|32differ"
	for name, o := range map[string]outcome{"c": c, "go": g} {
		if o.failed != "" || o.stdout != want {
			t.Errorf("%s:\n got %q\nwant %q\n%s", name, o.stdout, want, o.failed)
		}
	}
}
