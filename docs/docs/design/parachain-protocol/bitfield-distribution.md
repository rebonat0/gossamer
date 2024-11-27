# Bitfield Distribution Subsystem

[The bitfield distribution subsystem](https://paritytech.github.io/polkadot-sdk/book/node/availability/bitfield-distribution.html)
is responsible for gossipping signed availability bitfields. The bitfields express which parachain block candidates the
signing validator considers available.

## Subsystem Structure

The implementation must conform to the `Subsystem` interface defined in the `parachaintypes` package. It should live in
a package named `bitfielddistribution` under `dot/parachain/bitfield-distribution`.

### Messages Received

The subsystem must be registered with the overseer and handle two subsystem-specific messages from it:

1. [`validationprotocol.BitfieldDistributionMessage`](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/protocol/src/lib.rs#L621)

A network message containing a bitfield. This message is [identical in protocol versions 2 and 3](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/protocol/src/lib.rs#L870).
It is sufficient to only support those versions. Since the message was received from the network, the bitfield is unchecked.

2. [`DistributeBitfield`](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/subsystem-types/src/messages.rs#L522)

Gossip a bitfield to peers and announce it to other subsystems. The content of this message is the same as the message
under 1. but the bitfield has been checked or signed by this host if it is a validator. It needs to be converted to a
network message as part of the message handling code.

(Note: I'm not sure what all the sources for this message are. I assume the main source is the bitfield signing
subsystem.)

```go
package bitfielddistribution

import (
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	"github.com/ChainSafe/gossamer/lib/common"
)

type DistributeBitfield struct {
	RelayParent common.Hash
	Bitfield    parachaintypes.UncheckedSignedAvailabilityBitfield
}
```

It might make sense to duplicate the type `UncheckedSignedAvailabilityBitfield` as `CheckedSignedAvailabilityBitfield`,
add a method `ToChecked()` on `UncheckedSignedAvailabilityBitfield` that performs the validation and use that type in
`DistributeBitfield` to ensure only valid bitfields are sent to peers.

Additionally, the subsystem must handle the following general network bridge events and overseer signals:

1. `networkbridgevent.PeerConnected`
2. `networkbridgevent.PeerDisconnected`
3. `networkbridgevent.NewGossipTopology`
4. `networkbridgevent.PeerViewChange`
5. `networkbridgevent.OurViewChange`
6. `networkbridgevent.UpdatedAuthorityIDs`
7. `parachaintypes.ActiveLeavesUpdateSignal`

The overseer must be modified to forward these messages to the subsystem.

### Messages Sent

When handling `DistributeBitfield` messages, the subsystem sends a `ProvisionableDataBitfield` message to the overseer.
This type should be added in `dot/parachain/provisioner/messages/messages.go` in the following way:

```go
package provisionermessages

import (
	parachaintypes "github.com/ChainSafe/gossamer/dot/parachain/types"
	"github.com/ChainSafe/gossamer/lib/common"
)

var (
	// ...
	_ Data = (*ProvisionableDataBitfield)(nil)
)

// ...

type ProvisionableDataBitfield struct {
	RelayParent common.Hash
	Bitfield    parachaintypes.UncheckedSignedAvailabilityBitfield
}

func (ProvisionableDataBitfield) IsData() {}
```

If the type `CheckedSignedAvailabilityBitfield` mentioned above is created, it should also be used in
`ProvisionableDataBitfield`.

The subsystem also sends `networkbridgemessages.ReportPeer` during handling of various messages.

## Subsystem State

The subsystem should store the view of each peer the subsystem is informed about via the relevant network bridge events.
The Parity node also stores [the protocol version of the peer](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/bitfield-distribution/src/lib.rs#L135).
If our implementation only supports version 2 and 3 messages, the subsystem should probably ignore peers that still use
version 1 instead.

The subsystem also needs to know the current and previous network grid topologies and the view of the node ("our view").

[For each relay parent](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/bitfield-distribution/src/lib.rs#L161)
the subsystem is instructed by the overseer to work on, it needs to maintain the following data:
- the signing context (retrieved from runtime once)
- the validator set (retrieved from runtime once)
- any valid bitfield messages received from a validator (we can probably just store the network messages, since it's v2/v3 only)
- messages sent to a peer for this relay parent (store the validator ID instead of the message since there can only be one per validator)
- messages received from a peer for this relay parent (again, store validator ID, this is to avoid sending this peer a message for this validator)

## Message Handling Logic

- [`validationprotocol.BitfieldDistributionMessage`](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/bitfield-distribution/src/lib.rs#L519)

- [`DistributeBitfield`](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/bitfield-distribution/src/lib.rs#L350)

- [`networkbridgevent.PeerConnected`](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/bitfield-distribution/src/lib.rs#L712)

Only add peers with protocol version 2 or 3.

- [`networkbridgevent.PeerDisconnected`](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/bitfield-distribution/src/lib.rs#L720)

- [`networkbridgevent.NewGossipTopology`](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/bitfield-distribution/src/lib.rs#L725)

- [`networkbridgevent.PeerViewChange`](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/bitfield-distribution/src/lib.rs#L765)

- [`networkbridgevent.OurViewChange`](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/bitfield-distribution/src/lib.rs#L787)

- [`networkbridgevent.UpdatedAuthorityIDs`](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/bitfield-distribution/src/lib.rs#L777)

- [`parachaintypes.ActiveLeavesUpdateSignal`](https://github.com/paritytech/polkadot-sdk/blob/1e3b8e1639c1cf784eabf0a9afcab1f3987e0ca4/polkadot/node/network/bitfield-distribution/src/lib.rs#L289)
