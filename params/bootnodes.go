// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

var MainnetBootnodes = []string{
	"enode://c1978a713b5956b4f28f93905bf608a2b81817cdea534ecc3cc5e181f5e06931fe7a855178262a5a45f3c9ac49bae3c12a09ebaa57deaca30311133ec186a410@47.242.8.8:21212",
	"enode://b49c6ecf29dcf26cbf550c443be51a82f1672a320fb508c26c0764e5f87de97269cc1a82425d6a03f1109dc0a1c99bb689811226f52cc16d6e2e90260cc18875@8.210.177.221:21212",
	"enode://fc9637e1d88b15cfe0e2e84dd32015808be82047997d1fc2a66570f9e5c930858908663499eb03c1efdedb915df5d05bbcb29de2ee28f7e44be7700d42660d1a@47.242.33.241:21212",
	"enode://5cf6cc1eb8d82f7a7121c20d703557c271c9f852323f9b3c24a7ce1141b23ba469eeb74e349a8631805e26be51f4c8b0280fb98ae755b6df90d6bbfaf668b332@47.242.32.245:21212",
}

var MainnetInitIds = []string{
	"cbc19fe033c20712",
	"4750f91ec851cf19",
	"1f12e62cc0a5b083",
	"96aed4572b6a9aad",
	"ee300cfe9ec8e711",
	"b879e3d1b75cacb8",
	"fb4b08cc234140da",
	"09370fd23ded245c",
	"d1112f47fa7f4603",
	"9a6a6533620f022a",
	"8689b1cd6a7506b8",
	"a148a69437a74718",
	"afc11905ddb0e253",
	"f361e96f0be4f840",
	"1023e832a98e9a9e",
	"be5d4c78a0f3efdc",
	"147f25914d8b4006",
	"26e9adc5ce609546",
	"5c1a597fc24cbe01",
	"9531f74fe26179f2",
	"b492c74ec202a758",
}

var TestnetBootnodes = []string{
	"enode://59ca967b2c9c1442e81026f5ffc2b24f4b3787512194a41e4ab14dfac97e75b700988cac80f973641d40cd65f775f41955b93d2e843ebb03555b16dd9bf983d4@127.0.0.1:9646",
}

var MainnetMasternodes = []string{
	"enode://cbc19fe033c207127dd0cb3562d88e7ba32586b5daa466f43e8cdfb39c6fb2ccaa0687a524391212a58f7b1d556bbff774dd8eccf9b0b881ec3842dea3e9f8d9",
	"enode://4750f91ec851cf1927ee6107eaccbfea1cbf56f877d06e6037d580dc955e49d4100220269ee04133437e5fcfca4811e280dad8b0f49591c61b543798159c3c6c",
	"enode://1f12e62cc0a5b083cb09cf9105fed2ff49edbfdddd39e9a29e64f7b111edcab46e7cd927251450570c841055606013e7067e3a895853403804d08922fb69ea25",
	"enode://96aed4572b6a9aad548db1176636554c98ebf585c942717691dc71b066aec9408aa63628192c9da70bbc9601e54aae37360aba4f7d26929cab31b32865cca764",
	"enode://ee300cfe9ec8e711f9fc859e0154aaa3eca21525f6c8ac86ee6aa82511591adb1a6d8c1075d4024f47ccef449514ff347683aef03a0daee04e570e06e3b39771",
	"enode://b879e3d1b75cacb817c05f2a3cf34579dbfe12c2105a89afa26807a265f00a6681442c231c6e739b4d9000d1955d7bba692fc2cf79661b3635b4fa0ecc6af801",
	"enode://fb4b08cc234140dad7c3f2e76a3b83583acebdbc29129cf7fc10f9d002dc1e2cc7b831917425a1e0675b94b76d90995508cc8500448e0924e30f443a161f5804",
	"enode://09370fd23ded245cbce41baa9d3f9af0efc0e969024a54e6ddf4b1a494addc3ca4e445262e02fa96443d5b5d4970c993941159ccfadd700925c74455b45bddec",
	"enode://d1112f47fa7f460337039ec39a426fd94ade85a4b20dcfaecc9a237de368df59ec80be17fdc292a093f35deff63bff26fb5f0798cb20b508cf8c79931b830583",
	"enode://2aafeadb97f17f413d464d18235f56117060a56d42ff9a96414153e19887355405ebd3eef68a78ed8c672d31212c317b1aafa4d7e0cc020c6c7675e496f4f54d",
	"enode://9a6a6533620f022ab6549a7f345434b71214ff1f892be3619acc40e83072e1a6ca26f699f93b0c91e38acf646d3a5d212ff4ce84f8370f0a30d776a322863a3e",
	"enode://8689b1cd6a7506b8bbb54e2aa7a55ff0308cf8be26d132337b9e50f0c2cd709ece3e8650b83ce05e69ac9d78deb017cba76046c145dfd725103fd03580ffb083",
	"enode://a148a69437a74718e13d2b95fcb077f068e764ca7a117d806938db85810c329748da5f44c6b3faad4b5c897d8d1d9cda7a6408c825613da59f97062b75a92537",
	"enode://afc11905ddb0e253ad54f0ee31e08c9710fd3816103577bf01564692f91501c0708db2726c1fb7da9d162281d824d7474073a198def4fb24df50136289786402",
	"enode://f361e96f0be4f84052ce359e421de9f34e0a6673671cf627f89e9933b305d1a91e087cdf55e9fa22d68db7864e7e41ac24983246e92c769b0bc2322bbb91c09d",
	"enode://1023e832a98e9a9e30114112c04a47d98fbd548bcfa2715571128e0d4a345e39c58730ab622cbd00efb9cea3cf757e93de2e8e7d417adc4d74641a2dc01c4685",
	"enode://be5d4c78a0f3efdcc9b00a873c059c835a92b4b5b4ad89192fa07f80dad8bcfbf4fe3ec39f75a46806ff0216268cee44746244460deae7d6ed3f35eff434738b",
	"enode://147f25914d8b4006e4378e2488c3735658e4e4fb521740366407b8e4d0626c660279ee14fea9aba1149f9044c4f18e365183ef0fdccd048057615aba50b4f92a",
	"enode://26e9adc5ce609546ec4ca8744491efc6d8c8fb1cc92268514132e8a4f1fbb896876110487f7f098ed9355abb466b5922314b516df36d4f95da7ed4342a21a94e",
	"enode://5c1a597fc24cbe011ec3cfafd060ec2154ce7c0a66c5e73a2d564cfc6c6cec578af775c2c5074e344040c3c5ef50b3ad29f1cf9bb60e6736c7740ff8df026c40",
	"enode://9531f74fe26179f2ed3414db80c0a1fb8a1c1674d80437cb9420e9b0fc79011325d9a0a8f8e9611448e3566103fa2ada2a526f57f75910204753db37495bbd22",
	"enode://b492c74ec202a75827bd5ebb77c1d4a3c19111a5cd6964e41cc50f967352e23a265189452331fe01e5bcd73944258a77b24e205efd1ee32bda72535859cfaa06",
}

// RinkebyBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Rinkeby test network.
var RinkebyBootnodes = []string{}

// DiscoveryV5Bootnodes are the enode URLs of the P2P bootstrap nodes for the
// experimental RLPx v5 topic-discovery network.
var DiscoveryV5Bootnodes = []string{}
