# Changelog

## Unreleased


## 0.28.5 (Feb 7, 2022)

- Persist lottery identities ([#918])
- Make tx pool resetting optional in ceremony ([#919])

[#918]: https://github.com/idena-network/idena-go/pull/918
[#919]: https://github.com/idena-network/idena-go/pull/919

## 0.28.4 (Jan 18, 2022)

- Add an option to allow the node to turn on online mining status ([#907])
- Reload nonce cache fallback during ceremony ([#899])
- Update ipfs to 0.11.0 ([#900])
- Update rpc methods to interact with the profile ([#898])
- Handle panics of iavl tree while adding tx to mempool ([#895])

[#907]: https://github.com/idena-network/idena-go/pull/907
[#899]: https://github.com/idena-network/idena-go/pull/899
[#900]: https://github.com/idena-network/idena-go/pull/900
[#898]: https://github.com/idena-network/idena-go/pull/898
[#895]: https://github.com/idena-network/idena-go/pull/895

## 0.28.3 (Dec 28, 2021)

- Increase number of active peers ([#889])
- Fix extra delay in case of absence of proposer ([#888])
- Update iavl tree ([#890])
- Add method to provide identity state data with merkle proof ([#891])
- Add rpc method to estimate raw tx ([#885])

[#889]: https://github.com/idena-network/idena-go/pull/889
[#888]: https://github.com/idena-network/idena-go/pull/888
[#890]: https://github.com/idena-network/idena-go/pull/890
[#891]: https://github.com/idena-network/idena-go/pull/891
[#885]: https://github.com/idena-network/idena-go/pull/885


## 0.28.2 (Dec 7, 2021)

### Changes

- Add bootstrap node ([#873])
- Update deps  ([#877])
- Improve peering for multishard nodes ([#878])
- Reset tx pool in validation ceremony partially ([#879])

[#873]: https://github.com/idena-network/idena-go/pull/873
[#877]: https://github.com/idena-network/idena-go/pull/877
[#878]: https://github.com/idena-network/idena-go/pull/878
[#879]: https://github.com/idena-network/idena-go/pull/879

## 0.28.1 (Nov 15, 2021)

### Changes

- Fix offline penalty commitment ([#862])

[#862]: https://github.com/idena-network/idena-go/pull/862

## 0.28.0 (Nov 12, 2021)

### Fork (Upgrade 7)

- Update oracle voting ([#853](https://github.com/idena-network/idena-go/pull/853))
- Fix validation of ceremonial txs ([#848](https://github.com/idena-network/idena-go/pull/848))
- Fix rewards for cross-shard invites ([#846](https://github.com/idena-network/idena-go/pull/846))
- Fix report reward stake for newbies ([#845](https://github.com/idena-network/idena-go/pull/845))
- Fix calculation of shardId for candidates ([#843](https://github.com/idena-network/idena-go/pull/843))

### Changes

- Send reason of disconnection to peer ([#844](https://github.com/idena-network/idena-go/pull/844))
- Change boot node ([#842](https://github.com/idena-network/idena-go/pull/842))
- Use transaction mempool as entry holder for Push&Pull ([#849](https://github.com/idena-network/idena-go/pull/849))
- Update shared node config profile ([#847](https://github.com/idena-network/idena-go/pull/847))
- Update deps ([#838](https://github.com/idena-network/idena-go/pull/838))
- Fix burnt coins indexing ([#841](https://github.com/idena-network/idena-go/pull/841))
- Collect block proposals and vote counting info ([#851](https://github.com/idena-network/idena-go/pull/851))
- Calculate shardId for non validated addresses ([#850](https://github.com/idena-network/idena-go/pull/850))
- Use block with maximal hash when proposer proposes multiple blocks ([#852](https://github.com/idena-network/idena-go/pull/852))
- Increase file descriptor limit ([#854](https://github.com/idena-network/idena-go/pull/854))
- Fix shard iteration ([#855](https://github.com/idena-network/idena-go/pull/855))

## 0.27.4 (Oct 30, 2021)
### Changes
- Fix fast sync ([#839](https://github.com/idena-network/idena-go/pull/839))

## 0.27.3 (Oct 25, 2021)

### Changes

- Truncate blockchain to block #3417451 ([#818](https://github.com/idena-network/idena-go/pull/818))
- Implement Idena gossip 1.1.0 (support push and flip key
  batches) ([#821](https://github.com/idena-network/idena-go/pull/821))
- Add client type into open short session answers ([#822](https://github.com/idena-network/idena-go/pull/822) &
  [#832](https://github.com/idena-network/idena-go/pull/832))
- Respect flip keyPackages in evidence map ([#816](https://github.com/idena-network/idena-go/pull/816))
- Disable generation of fast sync snapshots v1 ([#827](https://github.com/idena-network/idena-go/pull/827))
- Broadcast open short answers 2 minutes after the start of validation and after successful long answers broadcast 
  ([#819](https://github.com/idena-network/idena-go/pull/819) & ([#828](https://github.com/idena-network/idena-go/pull/828)))
- Collect answers order and flag that flip is allowed to solve (
  indexer) ([#820](https://github.com/idena-network/idena-go/pull/820))
- Recover validation txs on after long session from tx keeper ([#831](https://github.com/idena-network/idena-go/pull/831))
- Update deps ([#830](https://github.com/idena-network/idena-go/pull/830) &
  [#778](https://github.com/idena-network/idena-go/pull/778) &
  [#826](https://github.com/idena-network/idena-go/pull/826) &
  [#806](https://github.com/idena-network/idena-go/pull/806) &
  [#808](https://github.com/idena-network/idena-go/pull/808) &
  [#801](https://github.com/idena-network/idena-go/pull/801) &
  [#824](https://github.com/idena-network/idena-go/pull/824) &
  [#823](https://github.com/idena-network/idena-go/pull/823))

## 0.27.2 (Oct 7, 2021)

### Changes

- Respect shardId in mempool syncing ([#805](https://github.com/idena-network/idena-go/pull/805))
- Update deps ([#776](https://github.com/idena-network/idena-go/pull/776)
  & [#796](https://github.com/idena-network/idena-go/pull/796))
- Add RPC method to estimate tx ([#809](https://github.com/idena-network/idena-go/pull/809))

## 0.27.1 (Sep 23, 2021)

### Changes

- Optimize network usage ([#802](https://github.com/idena-network/idena-go/pull/802))

## 0.27.0 (Sep 16, 2021)

### Fork (Upgrade 6)

- Validation sharding ([#695](https://github.com/idena-network/idena-go/pull/695))
- Introduce new keywords for flips ([#793](https://github.com/idena-network/idena-go/pull/793))
- Introduce separate fund to reward successful flip reports ([#792](https://github.com/idena-network/idena-go/pull/792))
- Change flip report thresholds for small committees ([#785](https://github.com/idena-network/idena-go/pull/785))
- Increase invites limit for the foundation address ([#781](https://github.com/idena-network/idena-go/pull/781))
- Fix killTx transaction validation ([#780](https://github.com/idena-network/idena-go/pull/780))
- Fix delegation for suspended and zombie ([#784](https://github.com/idena-network/idena-go/pull/784))

### Changes

- Add RPC method to fetch minimal required client version ([#786](https://github.com/idena-network/idena-go/pull/786))
- Restore mined transactions to mempool after blocks
  rollback ([#768](https://github.com/idena-network/idena-go/pull/768))
- Add mempool nonce into response of getBalance rpc method ([#779](https://github.com/idena-network/idena-go/pull/779))
- Add the time parameter when the node loads all flips for the validation to the node
  configuration ([#773](https://github.com/idena-network/idena-go/pull/773))
- Update boot nodes ([#787](https://github.com/idena-network/idena-go/pull/787))
- Add shared node profile ([#783](https://github.com/idena-network/idena-go/pull/783))

## 0.26.7 (09 Aug, 2021)

### Changes

- Implement async transaction keeper to prevent locks in p2p
  messaging ([#751](https://github.com/idena-network/idena-go/pull/751))
- Optimize transactions and flip keys validation ([#754](https://github.com/idena-network/idena-go/pull/754)
  & [#755](https://github.com/idena-network/idena-go/pull/755))
- Fix issue when public flip key could be missed ([#753](https://github.com/idena-network/idena-go/pull/753))
- Update deps ([#745](https://github.com/idena-network/idena-go/pull/745)
  & [#757](https://github.com/idena-network/idena-go/pull/757)
  & [#742](https://github.com/idena-network/idena-go/pull/742)
  & [#759](https://github.com/idena-network/idena-go/pull/759)
  & [#758](https://github.com/idena-network/idena-go/pull/758)
  & [#746](https://github.com/idena-network/idena-go/pull/746))
- Extend logs ([#756](https://github.com/idena-network/idena-go/pull/756))
- Use import/export API of iavl tree for state snapshot to reduce its
  size ([#764](https://github.com/idena-network/idena-go/pull/764))

## 0.26.6 (Jul 19, 2021)

### Changes

- Speed up fast sync ([#731](https://github.com/idena-network/idena-go/pull/731))
- Adjust consensus engine timeouts ([#732](https://github.com/idena-network/idena-go/pull/732))
- Disable noise protocol ([#733](https://github.com/idena-network/idena-go/pull/733))
- Keep all mempool transactions on disk ([#734](https://github.com/idena-network/idena-go/pull/734))
- Implement postponed validation of proposed blocks ([#735](https://github.com/idena-network/idena-go/pull/735))
- Publish ipfs peers info to eventbus ([#736](https://github.com/idena-network/idena-go/pull/736))

## 0.26.5 (Jul 4, 2021)

### Changes

- Revert go-ipfs from 0.9.0 to 0.8.0 ([#725](https://github.com/idena-network/idena-go/pull/725))

## 0.26.4 (Jul 3, 2021)

### Changes

- Optimize mempool sync ([#721](https://github.com/idena-network/idena-go/pull/721))
- Update consensus engine timeouts ([#720](https://github.com/idena-network/idena-go/pull/720))
- Extend identity API ([#718](https://github.com/idena-network/idena-go/pull/718))
- Update deps ([#717](https://github.com/idena-network/idena-go/pull/717)
  & [#716](https://github.com/idena-network/idena-go/pull/716)
  & [#710](https://github.com/idena-network/idena-go/pull/710)
  & [#719](https://github.com/idena-network/idena-go/pull/719)
  & [#714](https://github.com/idena-network/idena-go/pull/714)
  & [#715](https://github.com/idena-network/idena-go/pull/715)
  & [#722](https://github.com/idena-network/idena-go/pull/722))

## 0.26.3 (Jun 11, 2021)

### Breaking Changes

- Remove `currentValidationStart` from response of `dna_epoch` RPC
  method ([#706](https://github.com/idena-network/idena-go/pull/706))

### Changes

- Get rid of double block processing while inserting a block into the
  chain ([#704](https://github.com/idena-network/idena-go/pull/704)
  & [#708](https://github.com/idena-network/idena-go/pull/708))
- Add `startBlock` to response of `dna_epoch` RPC method ([#706](https://github.com/idena-network/idena-go/pull/706))
- Upgrade to GitHub-native Dependabot ([#683](https://github.com/idena-network/idena-go/pull/683))
- Update iavl and protobuf deps ([#696](https://github.com/idena-network/idena-go/pull/696))
- Update deps ([#698](https://github.com/idena-network/idena-go/pull/698)
  & [#699](https://github.com/idena-network/idena-go/pull/699)
  & [#700](https://github.com/idena-network/idena-go/pull/700)
  & [#701](https://github.com/idena-network/idena-go/pull/701)
  & [#707](https://github.com/idena-network/idena-go/pull/707))
- Add CHANGELOG.md ([#702](https://github.com/idena-network/idena-go/pull/702))

## 0.26.2 (May 26, 2021)

### Bug Fixes

- Fix invitation reward calculation ([#692](https://github.com/idena-network/idena-go/pull/692))

## 0.26.1 (May 11, 2021)

### Bug Fixes

- Fix offline detector flags validation ([#686](https://github.com/idena-network/idena-go/pull/686))

## 0.26.0 (May 7, 2021)

### Fork (Upgrade 5)

- Encourage early invitations ([#671](https://github.com/idena-network/idena-go/pull/671))
- Check 5 sequential blocks without ceremonial txs to finish the validation
  ceremony ([#672](https://github.com/idena-network/idena-go/pull/672))
- Implement delayed offline penalties ([#674](https://github.com/idena-network/idena-go/pull/674))
- Burn 5/6 of invitee stake in KillInviteeTx ([#676](https://github.com/idena-network/idena-go/pull/676))

### Changes

- Add new tx type to store data to IPFS ([#670](https://github.com/idena-network/idena-go/pull/670))

### Bug Fixes

- Fix events for pool rewards in oracle voting ([#669](https://github.com/idena-network/idena-go/pull/669))

## 0.25.3 (Apr 30, 2021)

### Changes

- Save own mempool transactions to disk ([#675](https://github.com/idena-network/idena-go/pull/675)
  & [#679](https://github.com/idena-network/idena-go/pull/679)
  & [#680](https://github.com/idena-network/idena-go/pull/680)
  & [#681](https://github.com/idena-network/idena-go/pull/681))
- Add flip keywords API ([#678](https://github.com/idena-network/idena-go/pull/678))

### Bug Fixes

- Fix data races ([#667](https://github.com/idena-network/idena-go/pull/667))
- Trim whitespace around API key
  from [@busimus](https://github.com/busimus) ([#668](https://github.com/idena-network/idena-go/pull/668))

## 0.25.2 (Apr 1, 2021)

Finish voting transactions will be enabled on the next seamless fork.

### Changes

- Temporary disable finish oracle voting transactions due to
  bug ([#665](https://github.com/idena-network/idena-go/pull/665))

## 0.25.1 (Mar 27, 2021)

### Changes

- Respect delegation switch in RPC calls ([#657](https://github.com/idena-network/idena-go/pull/657))

## 0.25.0 (Mar 22, 2021)

### Fork (Upgrade 4)

- Introduce pools ([#631](https://github.com/idena-network/idena-go/pull/631))
- Update contracts ([#649](https://github.com/idena-network/idena-go/pull/649))
- Increase flip rewards, decrease invite rewards ([#651](https://github.com/idena-network/idena-go/pull/651))
- Burn 5/6 of invitee stake in KillInviteeTx ([#676](https://github.com/idena-network/idena-go/pull/676))

### Changes

- Increase file descriptor limit ([#653](https://github.com/idena-network/idena-go/pull/653))
- Update deps ([#645](https://github.com/idena-network/idena-go/pull/645)
  & [#648](https://github.com/idena-network/idena-go/pull/648))

### Bug Fixes

- Fix build on linux ([#650](https://github.com/idena-network/idena-go/pull/650)
  & [#654](https://github.com/idena-network/idena-go/pull/654))
