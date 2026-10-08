# Networking

A program that speaks MQTT, HTTP or a websocket should not carry a vendor's
networking stack in its source. `robohub-common` declares the contract such a
program is written against — `robohub/net.h` — and each platform implements it.
A protocol built on this compiles for a Pico W, an ESP32 or a Linux gateway
without a line changed.

## The contract

```c
/* Interface */
RoboHubError     robohub_net_wifi_connect(const RoboHubWiFiConfig* config);
RoboHubError     robohub_net_wifi_disconnect(void);
RoboHubNetStatus robohub_net_status(void);
RoboHubError     robohub_net_ip(char* out_ip, rh_size max_len);

/* Sockets */
RoboHubSocket robohub_socket_create(RoboHubSocketType type);
RoboHubError  robohub_socket_connect(RoboHubSocket socket, const char* host, rh_u16 port);
int           robohub_socket_send(RoboHubSocket socket, const void* data, rh_size len);
int           robohub_socket_recv(RoboHubSocket socket, void* data, rh_size len, rh_u32 timeout_ms);
void          robohub_socket_close(RoboHubSocket socket);
```

Every function has a **weak default** in `robohub-common`. A board with no network
links and runs: the default answers `ROBOHUB_ERR_NOT_SUPPORTED`, returns a null
socket, or reports a disconnected interface. A platform that implements the
contract defines these functions strongly and the linker prefers them — the same
mechanism the OSAL uses to choose an RTOS.

`recv` takes a timeout and must not block longer than it says. `ROBOHUB_SOCKET_TIMEOUT`
is returned when the time passed with nothing to read — a poll that came up empty,
not an error. A platform implements the wait as a non-blocking poll between OSAL
sleeps, so a network read never starves the control loop sharing its core.

## How each platform supplies it

| Capability | ESP32 | Pico W | Linux / gateway |
|------------|-------|--------|-----------------|
| Interface control | `esp_wifi` + `esp_event` | `cyw43_arch_wifi_connect_timeout_ms` | NetworkManager / `wpa_supplicant` |
| IP stack | LwIP via `esp_netif` | LwIP via the Pico SDK | POSIX |
| Sockets | LwIP BSD wrapper | LwIP RAW API (`tcp`/`udp` pcbs) | `<sys/socket.h>` |
| Name resolution | `getaddrinfo` (LwIP) | numeric IPv4 (`ip4addr_aton`) | `getaddrinfo` |

The Pico W column is not the BSD layer because its arch —
`pico_cyw43_arch_lwip_threadsafe_background` — runs LwIP bare-metal
(`NO_SYS=1`), and the BSD socket and netconn layers need an OS to block in. The
platform's `pico_net.c` therefore answers the socket contract over the RAW API,
with a single-producer/single-consumer ring the lwIP callbacks fill and a caller
drains; that is also what makes a receive the non-blocking poll the contract
asks for. Resolving a name would need `dns_gethostbyname`'s asynchronous
callback, so `connect` takes a numeric address, as the POSIX reference does.

The POSIX column is the reference: `tests/test_net_posix.c` implements the
contract over `<sys/socket.h>` and drives it against a loopback socket, so the
contract itself is exercised without a board. A platform port follows the same
shape with its own stack beneath.

## Enabling it

A project selects the contract with `RH_NET`; a platform's build then compiles
its implementation. A project that never opens a socket does not select it, and
pays nothing.
