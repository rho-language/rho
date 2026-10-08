# The RoboHub stack

How the pieces fit, and how a reading travels from a sensor to a gateway.

```
                            rho protocol  (IDL)
                    one schema ──┬───────────────┬──
                                 │ C packed       │ Go JSON + HTTP
                                 ▼                ▼
┌───────────────────────────────────────────┐   ┌────────────────────┐
│              robohub-core                │   │  Go edge / cloud   │
│   event bus · scheduler · apps · diag    │   └────────────────────┘
└───────────────▲──────────────────────────┘
                │ publish / subscribe
      ┌─────────┴──────────┐
      │  robohub-telemetry │  filter · decimate · route · logs
      └───┬────────────┬───┘
          ▼            ▼
 ┌────────────────┐ ┌──────────────┐
 │robohub-transport│ │ robohub-net  │
 │ COBS · CRC16    │ │ sockets/IP   │
 └───┬────────────┘ └──────┬───────┘
     ▼                     ▼
 USB CDC / UART / RS-485   TCP / UDP / Wi-Fi
```

## The modules

| Module | What it is |
|--------|------------|
| `robohub-common` | The contracts: HAL (gpio, i2c, spi, uart, rs485, timer, onewire), driver interfaces (`temperature`), the OSAL, and `net`. Weak defaults where a platform may be absent. |
| `robohub-core` | The application runtime: the event bus, the scheduler, the app lifecycle, diagnostics. Bare-metal OSAL defaults. |
| `robohub-transport` | Turns a byte stream into validated frames: COBS, Fletcher-16, a byte-at-a-time parser. No allocation. |
| `robohub-net` | A target-agnostic interface and sockets. The contract lives in `robohub-common`; each platform implements it. |
| `robohub-telemetry` | Binds the event bus to sinks (transport, socket) with per-event decimation and log interception. |
| `robohub-freertos` | Middleware: the OSAL over FreeRTOS. |
| `robohub-drivers` | Device drivers (`DS18B20`), eval module boards, and the examples. |
| `robohub-pico`, `robohub-esp32`, `robohub-stm32` | Platforms: the HAL, board support packages, and the network port. |

## One reading, end to end

The DS18B20 driver publishes `EVENT_TEMP_READING`; it names no transport.

1. `robohub-core` carries the event to its subscribers.
2. `robohub-telemetry`, subscribed to that event, writes the payload to every
   enabled sink — a transport and, at once, a socket if one is registered.
3. `robohub-transport` frames it: `msg_id · tx_id · payload_len · payload · crc16`,
   little-endian, COBS-encoded, `0x00`-delimited.
4. The link writes the bytes — USB CDC here.

On the board's console the bytes are a frame, decoded by a host into
`msg=0x0201 tx=0 len=4 temp=24.31 C`. The payload is the packed struct a
`rho protocol` schema generates, so the Go side decodes the same bytes into the
same value.

## The rule that holds it together

Every platform-dependent thing has a contract and a weak default low in the tree
(the OSAL in core, the network in `common`), and a platform or middleware
supplies the strong definition. A program is the same whether the RTOS, the
network or the sensor is there — and a board that has none of them still links.

## What is complete, and what is not

- Complete: the IDL, the runtime, transport, telemetry, the OSAL and its
  FreeRTOS middleware, the DS18B20 driver and its board, and the Pico 2W HAL and
  console. The whole stack streams a reading over USB CDC.
- Platform network ports: the POSIX implementation exists as the reference and
  the test; the ESP32 and Pico W ports are the next piece, each over its own
  stack (`esp_netif`/`esp_wifi`, `cyw43_arch` + LwIP).
