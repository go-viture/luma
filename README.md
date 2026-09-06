# go-viture/luma

Talk to a **VITURE Luma Ultra** headset from pure Go, `CGO_ENABLED=0`, with no
vendor library.

```go
g, err := luma.Open()
if err != nil { return err }
defer g.Close()

v, err := g.Version()      // 02 01 09 on the headset this was written for
```

## What is proven

Measured on real hardware, 2026-09-06:

```
write → endpoint 0x04        read ← endpoint 0x85        device 35ca:1104

fa 55 80 00 00   →   fa 55 80 00 00 02 01 09
fa 55 c3 00 00   →   fa 55 c3 01 00 00
```

The envelope:

    FA 55 | command | length (16-bit big-endian) | payload

⭐ **The headset echoes the command byte**, so replies match requests without a
sequence number — and that is worth checking, because the inbound pipe is
shared and a late answer to an earlier question has the right shape and arrives
at the right moment.

⛔ **Drain before every command.** Not housekeeping: the vendor's own code
clears the inbound pipe before each one, for exactly that reason. `Ask` does it.

## ⛔ The headset never speaks first

Measured over 75 seconds, with the buttons on the arm pressed and the head
moving, listening on all five inbound pipes: **twelve identical frames in the
first ten milliseconds** — a buffer flushed at open — and then nothing at all.

It is a request-and-response device. **No amount of listening decodes it.**
That is the opposite of the VITURE Beast, which announces what its own buttons
do; the two do not even share an envelope, so do not carry habits across.

## ⚠ What is NOT settled

**The length field of a reply.**

```
0x80 → "00 00" then THREE bytes
0xc3 → "01 00" then ONE byte
```

Read little-endian the second works and the first does not. `Reply` therefore
hands back those two bytes **and** everything after them, unparsed, rather than
pretending. Guessing here is how nineteen attempts were lost on the sibling
headset, which answers in one byte order and accepts commands in another.

## ⛔ Two commands, and twenty-five deliberately absent

`0x80` and `0xc3` are the frames VITURE's own `carina_a1088_get_firmware_version`
sends — which is why they were safe to try first: **a version read changes
nothing, repeats safely, and its success is visible.**

Twenty-five other command bytes are known to exist. None is here. Knowing a
number is not knowing what it does, and an unknown opcode written to a headset
somebody is wearing is not a probe.

## Requirements

- macOS for the transport; the frame model builds and is tested everywhere.
- **VITURE's SpaceWalker must be closed.** It holds the vendor interface while
  it runs, and `Open` then fails with the system's exclusive-access error.

## Provenance

The envelope and the endpoints were read out of `libcarina_vio.dylib`, the
vendor's own library, shipped inside SpaceWalker.app. Interoperability is a
recognised purpose for that (EU Directive 2009/24/EC, Article 6). Nothing was
copied: this is an independent implementation of an interface.

## Licence

BSD-3-Clause.
