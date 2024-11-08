# Running an OP Mainnet or Testnet node

## Non-docker configuration

Here are instructions if you want to run boba erigon version as the replica node for OP Mainnet or Testnet via binary files.

### Build erigon

1. Clone [boba erigon](https://github.com/bobanetwork/v3-erigon) repo

	```bash
	cd ~
	git clone https://github.com/bobanetwork/v3-erigon.git
	```

2. Build [boba erigon](https://github.com/bobanetwork/v3-erigon)

	```bash
	cd v3-erigon
	make erigon
	```

### Build op-node

1. Clone [boba anchorage](https://github.com/bobanetwork/v3-anchorage) repo

   ```bash
   cd ~
   git clone https://github.com/bobanetwork/v3-anchorage.git
   ```

2. Build [op-node](https://github.com/bobanetwork/v3-anchorage/tree/boba-develop/op-node)

	```bash
	cd v3-anchorage/op-node
	make
	```

### Get the data dir

The next step is to download the initial data for `op-erigon`. Thanks for the contribution from [Test in Prod](https://www.testinprod.io).

1. Download the correct data directory snapshot.
   * [OP Mainnet](https://op-erigon-backup.mainnet.testinprod.io)
   * [OP Goerli](https://op-erigon-backup.goerli.testinprod.io)

2. Create the data directory in `op-erigon` and fill it.

   ```bash
   mkdir op-erigon
   cd ./op-erigon
   mkdir erigon
   cd ./erigon
   tar xvf ~/[DIR]/op-erigon-goerli.tar
   ```

3. Create a shared secret (JWT token)

   ```bash
   cd op-erigon
   openssl rand -hex 32 > jwt.txt
   ```

### Scripts to start the different components

`op-erigon`

```bash
cd v3-erigon
./build/bin/erigon \
--datadir=DATA_DIRECTORY \
--private.api.addr=localhost:9090 \
--http.addr=0.0.0.0 \
--http.port=9545 \
--http.corsdomain="*" \
--http.vhosts="*" \
--authrpc.addr=0.0.0.0 \
--authrpc.port=8551 \
--authrpc.vhosts="*" \
--authrpc.jwtsecret=JWT_TOKEN_PATH \
--rollup.disabletxpoolgossip=true \
--chain=optimism-goerli \
--nodiscover \
```

`op-node`

```bash
cd v3-anchorage/op-node
./bin/op-node \
--l1=https://ethereum-goerli.publicnode.com \
--l2=http://localhost:8551 \
--l2.jwt-secret=JWT_TOKEN_PATH \
--network=goerli \
--rpc.addr=localhost \
--rpc.port=8547 \
--l1.http-poll-interval=500ms \
--l1.trustrpc=true \
--p2p.disable=true \
```

### The initial synchornization

During the initial synchonization, you get log messages from `op-node`, and nothing else appears to happen.

```bash
INFO [08-04|16:36:07.150] Advancing bq origin                      origin=df76ff..48987e:8301316 originBehind=false
```

After a few minutes, `op-node` finds the right batch and then it starts synchronizing. During this synchonization process, you get log messags from `op-node`.

```bash
INFO [08-04|16:36:01.204] Found next batch                         epoch=44e203..fef9a5:8301309 batch_epoch=8301309                batch_timestamp=1,673,567,518
INFO [08-04|16:36:01.205] generated attributes in payload queue    txs=2  timestamp=1,673,567,518
INFO [08-04|16:36:01.265] inserted block                           hash=ee61ee..256300 number=4,069,725 state_root=a582ae..33a7c5 timestamp=1,673,567,518 parent=5b102e..13196c prev_randao=4758ca..11ff3a fee_recipient=0x4200000000000000000000000000000000000011 txs=2  update_safe=true
```

### Note

It must run `boba-erigon` first and shut it down last.