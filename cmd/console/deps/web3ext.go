package deps

var Modules = map[string]string{
	"admin":      Admin_JS,
	"chequebook": Chequebook_JS,
	"clique":     Clique_JS,
	"ethash":     Ethash_JS,
	"debug":      Debug_JS,
	"entropy":    Entropy_JS,
	"miner":      Miner_JS,
	"net":        Net_JS,
	"personal":   Personal_JS,
	"rpc":        RPC_JS,
	"shh":        Shh_JS,
	"swarmfs":    SWARMFS_JS,
	"txpool":     TxPool_JS,
}

const Chequebook_JS = `
entropy3._extend({
	property: 'chequebook',
	methods: [
		new entropy3._extend.Method({
			name: 'deposit',
			call: 'chequebook_deposit',
			params: 1,
			inputFormatter: [null]
		}),
		new entropy3._extend.Property({
			name: 'balance',
			getter: 'chequebook_balance',
			outputFormatter: entropy3._extend.utils.toDecimal
		}),
		new entropy3._extend.Method({
			name: 'cash',
			call: 'chequebook_cash',
			params: 1,
			inputFormatter: [null]
		}),
		new entropy3._extend.Method({
			name: 'issue',
			call: 'chequebook_issue',
			params: 2,
			inputFormatter: [null, null]
		}),
	]
});
`

const Clique_JS = `
entropy3._extend({
	property: 'clique',
	methods: [
		new entropy3._extend.Method({
			name: 'getSnapshot',
			call: 'clique_getSnapshot',
			params: 1,
			inputFormatter: [null]
		}),
		new entropy3._extend.Method({
			name: 'getSnapshotAtHash',
			call: 'clique_getSnapshotAtHash',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'getSigners',
			call: 'clique_getSigners',
			params: 1,
			inputFormatter: [null]
		}),
		new entropy3._extend.Method({
			name: 'getSignersAtHash',
			call: 'clique_getSignersAtHash',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'propose',
			call: 'clique_propose',
			params: 2
		}),
		new entropy3._extend.Method({
			name: 'discard',
			call: 'clique_discard',
			params: 1
		}),
	],
	properties: [
		new entropy3._extend.Property({
			name: 'proposals',
			getter: 'clique_proposals'
		}),
	]
});
`

const Ethash_JS = `
entropy3._extend({
	property: 'ethash',
	methods: [
		new entropy3._extend.Method({
			name: 'getWork',
			call: 'ethash_getWork',
			params: 0
		}),
		new entropy3._extend.Method({
			name: 'getHashrate',
			call: 'ethash_getHashrate',
			params: 0
		}),
		new entropy3._extend.Method({
			name: 'submitWork',
			call: 'ethash_submitWork',
			params: 3,
		}),
		new entropy3._extend.Method({
			name: 'submitHashRate',
			call: 'ethash_submitHashRate',
			params: 2,
		}),
	]
});
`

const Admin_JS = `
entropy3._extend({
	property: 'admin',
	methods: [
		new entropy3._extend.Method({
			name: 'addPeer',
			call: 'admin_addPeer',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'removePeer',
			call: 'admin_removePeer',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'addTrustedPeer',
			call: 'admin_addTrustedPeer',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'removeTrustedPeer',
			call: 'admin_removeTrustedPeer',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'exportChain',
			call: 'admin_exportChain',
			params: 1,
			inputFormatter: [null]
		}),
		new entropy3._extend.Method({
			name: 'importChain',
			call: 'admin_importChain',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'sleepBlocks',
			call: 'admin_sleepBlocks',
			params: 2
		}),
		new entropy3._extend.Method({
			name: 'startRPC',
			call: 'admin_startRPC',
			params: 4,
			inputFormatter: [null, null, null, null]
		}),
		new entropy3._extend.Method({
			name: 'stopRPC',
			call: 'admin_stopRPC'
		}),
		new entropy3._extend.Method({
			name: 'startWS',
			call: 'admin_startWS',
			params: 4,
			inputFormatter: [null, null, null, null]
		}),
		new entropy3._extend.Method({
			name: 'stopWS',
			call: 'admin_stopWS'
		}),
	],
	properties: [
		new entropy3._extend.Property({
			name: 'nodeInfo',
			getter: 'admin_nodeInfo'
		}),
		new entropy3._extend.Property({
			name: 'peers',
			getter: 'admin_peers'
		}),
		new entropy3._extend.Property({
			name: 'datadir',
			getter: 'admin_datadir'
		}),
	]
});
`

const Debug_JS = `
entropy3._extend({
	property: 'debug',
	methods: [
		new entropy3._extend.Method({
			name: 'printBlock',
			call: 'debug_printBlock',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'getBlockRlp',
			call: 'debug_getBlockRlp',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'setHead',
			call: 'debug_setHead',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'seedHash',
			call: 'debug_seedHash',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'dumpBlock',
			call: 'debug_dumpBlock',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'chaindbProperty',
			call: 'debug_chaindbProperty',
			params: 1,
			outputFormatter: console.log
		}),
		new entropy3._extend.Method({
			name: 'chaindbCompact',
			call: 'debug_chaindbCompact',
		}),
		new entropy3._extend.Method({
			name: 'metrics',
			call: 'debug_metrics',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'verbosity',
			call: 'debug_verbosity',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'vmodule',
			call: 'debug_vmodule',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'backtraceAt',
			call: 'debug_backtraceAt',
			params: 1,
		}),
		new entropy3._extend.Method({
			name: 'stacks',
			call: 'debug_stacks',
			params: 0,
			outputFormatter: console.log
		}),
		new entropy3._extend.Method({
			name: 'freeOSMemory',
			call: 'debug_freeOSMemory',
			params: 0,
		}),
		new entropy3._extend.Method({
			name: 'setGCPercent',
			call: 'debug_setGCPercent',
			params: 1,
		}),
		new entropy3._extend.Method({
			name: 'memStats',
			call: 'debug_memStats',
			params: 0,
		}),
		new entropy3._extend.Method({
			name: 'gcStats',
			call: 'debug_gcStats',
			params: 0,
		}),
		new entropy3._extend.Method({
			name: 'cpuProfile',
			call: 'debug_cpuProfile',
			params: 2
		}),
		new entropy3._extend.Method({
			name: 'startCPUProfile',
			call: 'debug_startCPUProfile',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'stopCPUProfile',
			call: 'debug_stopCPUProfile',
			params: 0
		}),
		new entropy3._extend.Method({
			name: 'goTrace',
			call: 'debug_goTrace',
			params: 2
		}),
		new entropy3._extend.Method({
			name: 'startGoTrace',
			call: 'debug_startGoTrace',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'stopGoTrace',
			call: 'debug_stopGoTrace',
			params: 0
		}),
		new entropy3._extend.Method({
			name: 'blockProfile',
			call: 'debug_blockProfile',
			params: 2
		}),
		new entropy3._extend.Method({
			name: 'setBlockProfileRate',
			call: 'debug_setBlockProfileRate',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'writeBlockProfile',
			call: 'debug_writeBlockProfile',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'mutexProfile',
			call: 'debug_mutexProfile',
			params: 2
		}),
		new entropy3._extend.Method({
			name: 'setMutexProfileFraction',
			call: 'debug_setMutexProfileFraction',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'writeMutexProfile',
			call: 'debug_writeMutexProfile',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'writeMemProfile',
			call: 'debug_writeMemProfile',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'traceBlock',
			call: 'debug_traceBlock',
			params: 2,
			inputFormatter: [null, null]
		}),
		new entropy3._extend.Method({
			name: 'traceBlockFromFile',
			call: 'debug_traceBlockFromFile',
			params: 2,
			inputFormatter: [null, null]
		}),
		new entropy3._extend.Method({
			name: 'traceBlockByNumber',
			call: 'debug_traceBlockByNumber',
			params: 2,
			inputFormatter: [null, null]
		}),
		new entropy3._extend.Method({
			name: 'traceBlockByHash',
			call: 'debug_traceBlockByHash',
			params: 2,
			inputFormatter: [null, null]
		}),
		new entropy3._extend.Method({
			name: 'traceTransaction',
			call: 'debug_traceTransaction',
			params: 2,
			inputFormatter: [null, null]
		}),
		new entropy3._extend.Method({
			name: 'preimage',
			call: 'debug_preimage',
			params: 1,
			inputFormatter: [null]
		}),
		new entropy3._extend.Method({
			name: 'getBadBlocks',
			call: 'debug_getBadBlocks',
			params: 0,
		}),
		new entropy3._extend.Method({
			name: 'storageRangeAt',
			call: 'debug_storageRangeAt',
			params: 5,
		}),
		new entropy3._extend.Method({
			name: 'getModifiedAccountsByNumber',
			call: 'debug_getModifiedAccountsByNumber',
			params: 2,
			inputFormatter: [null, null],
		}),
		new entropy3._extend.Method({
			name: 'getModifiedAccountsByHash',
			call: 'debug_getModifiedAccountsByHash',
			params: 2,
			inputFormatter:[null, null],
		}),
	],
	properties: []
});
`

const Entropy_JS = `
entropy3._extend({
	property: 'entropy',
	methods: [
		new entropy3._extend.Method({
			name: 'sign',
			call: 'entropy_sign',
			params: 2,
			inputFormatter: [entropy3._extend.formatters.inputAddressFormatter, null]
		}),
		new entropy3._extend.Method({
			name: 'resend',
			call: 'entropy_resend',
			params: 3,
			inputFormatter: [entropy3._extend.formatters.inputTransactionFormatter, entropy3._extend.utils.fromDecimal, entropy3._extend.utils.fromDecimal]
		}),
		new entropy3._extend.Method({
			name: 'signTransaction',
			call: 'entropy_signTransaction',
			params: 1,
			inputFormatter: [entropy3._extend.formatters.inputTransactionFormatter]
		}),
		new entropy3._extend.Method({
			name: 'submitTransaction',
			call: 'entropy_submitTransaction',
			params: 1,
			inputFormatter: [entropy3._extend.formatters.inputTransactionFormatter]
		}),
		new entropy3._extend.Method({
			name: 'getRawTransaction',
			call: 'entropy_getRawTransactionByHash',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'getRawTransactionFromBlock',
			call: function(args) {
				return (entropy3._extend.utils.isString(args[0]) && args[0].indexOf('0x') === 0) ? 'entropy_getRawTransactionByBlockHashAndIndex' : 'entropy_getRawTransactionByBlockNumberAndIndex';
			},
			params: 2,
			inputFormatter: [entropy3._extend.formatters.inputBlockNumberFormatter, entropy3._extend.utils.toHex]
		}),
	],
	properties: [
		new entropy3._extend.Property({
			name: 'pendingTransactions',
			getter: 'entropy_pendingTransactions',
			outputFormatter: function(txs) {
				var formatted = [];
				for (var i = 0; i < txs.length; i++) {
					formatted.push(entropy3._extend.formatters.outputTransactionFormatter(txs[i]));
					formatted[i].blockHash = null;
				}
				return formatted;
			}
		}),
	]
});
`

const Miner_JS = `
entropy3._extend({
	property: 'miner',
	methods: [
		new entropy3._extend.Method({
			name: 'start',
			call: 'miner_start',
			params: 1,
			inputFormatter: [null]
		}),
		new entropy3._extend.Method({
			name: 'stop',
			call: 'miner_stop'
		}),
		new entropy3._extend.Method({
			name: 'setEtherbase',
			call: 'miner_setEtherbase',
			params: 1,
			inputFormatter: [entropy3._extend.formatters.inputAddressFormatter]
		}),
		new entropy3._extend.Method({
			name: 'setExtra',
			call: 'miner_setExtra',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'setGasPrice',
			call: 'miner_setGasPrice',
			params: 1,
			inputFormatter: [entropy3._extend.utils.fromDecimal]
		}),
		new entropy3._extend.Method({
			name: 'setRecommitInterval',
			call: 'miner_setRecommitInterval',
			params: 1,
		}),
		new entropy3._extend.Method({
			name: 'getHashrate',
			call: 'miner_getHashrate'
		}),
	],
	properties: []
});
`

const Net_JS = `
entropy3._extend({
	property: 'net',
	methods: [],
	properties: [
		new entropy3._extend.Property({
			name: 'version',
			getter: 'net_version'
		}),
	]
});
`

const Personal_JS = `
entropy3._extend({
	property: 'personal',
	methods: [
		new entropy3._extend.Method({
			name: 'importRawKey',
			call: 'personal_importRawKey',
			params: 2
		}),
		new entropy3._extend.Method({
			name: 'sign',
			call: 'personal_sign',
			params: 3,
			inputFormatter: [null, entropy3._extend.formatters.inputAddressFormatter, null]
		}),
		new entropy3._extend.Method({
			name: 'ecRecover',
			call: 'personal_ecRecover',
			params: 2
		}),
		new entropy3._extend.Method({
			name: 'openWallet',
			call: 'personal_openWallet',
			params: 2
		}),
		new entropy3._extend.Method({
			name: 'deriveAccount',
			call: 'personal_deriveAccount',
			params: 3
		}),
		new entropy3._extend.Method({
			name: 'signTransaction',
			call: 'personal_signTransaction',
			params: 2,
			inputFormatter: [entropy3._extend.formatters.inputTransactionFormatter, null]
		}),
	],
	properties: [
		new entropy3._extend.Property({
			name: 'listWallets',
			getter: 'personal_listWallets'
		}),
	]
})
`

const RPC_JS = `
entropy3._extend({
	property: 'rpc',
	methods: [],
	properties: [
		new entropy3._extend.Property({
			name: 'modules',
			getter: 'rpc_modules'
		}),
	]
});
`

const Shh_JS = `
entropy3._extend({
	property: 'shh',
	methods: [
	],
	properties:
	[
		new entropy3._extend.Property({
			name: 'version',
			getter: 'shh_version',
			outputFormatter: entropy3._extend.utils.toDecimal
		}),
		new entropy3._extend.Property({
			name: 'info',
			getter: 'shh_info'
		}),
	]
});
`

const SWARMFS_JS = `
entropy3._extend({
	property: 'swarmfs',
	methods:
	[
		new entropy3._extend.Method({
			name: 'mount',
			call: 'swarmfs_mount',
			params: 2
		}),
		new entropy3._extend.Method({
			name: 'unmount',
			call: 'swarmfs_unmount',
			params: 1
		}),
		new entropy3._extend.Method({
			name: 'listmounts',
			call: 'swarmfs_listmounts',
			params: 0
		}),
	]
});
`

const TxPool_JS = `
entropy3._extend({
	property: 'txpool',
	methods: [],
	properties:
	[
		new entropy3._extend.Property({
			name: 'content',
			getter: 'txpool_content'
		}),
		new entropy3._extend.Property({
			name: 'inspect',
			getter: 'txpool_inspect'
		}),
		new entropy3._extend.Property({
			name: 'status',
			getter: 'txpool_status',
			outputFormatter: function(status) {
				status.pending = entropy3._extend.utils.toDecimal(status.pending);
				status.queued = entropy3._extend.utils.toDecimal(status.queued);
				return status;
			}
		}),
	]
});
`