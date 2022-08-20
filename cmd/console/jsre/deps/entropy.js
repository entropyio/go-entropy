(function () {
    var send = function (req) {
        var result = jEntropy.send(req);
        if (result && result.result) {
            return result.result;
        } else if (result.error) {
            return result.error.message;
        }
        return "resultError"
    };

    var isString = function (object) {
        return typeof object === 'string' ||
            (object && object.constructor && object.constructor.name === 'String');
    };

    var toNumber = function (str) {
        var number = parseInt(str, 16);
        return (isNaN(number) ? 0 : number);
    };

    entropy = {
        // property
        version: function () {
            return 'Entropy Command Line V1.0.0';
        },

        coinbase: function () {
            return send({"method": "entropy_coinbase", "params": []});
        },

        mining: function () {
            return send({"method": "entropy_mining", "params": []});
        },

        hashrate: function () {
            return send({"method": "entropy_hashrate", "params": []});
        },

        syncing: function () {
            return send({"method": "entropy_syncing", "params": []});
        },

        gasPrice: function () {
            return send({"method": "entropy_gasPrice", "params": []});
        },

        accounts: function () {
            return send({"method": "entropy_accounts", "params": []});
        },

        blockNumber: function () {
            var result = send({"method": "entropy_blockNumber", "params": []});
            return toNumber(result);
        },

        protocolVersion: function () {
            return send({"method": "entropy_protocolVersion", "params": []});
        },

        // methods
        getBalance: function (address, blockNumber) {
            if (typeof blockNumber == "undefined") {
                blockNumber = "latest"
            }

            var result = send({"method": "entropy_getBalance", "params": [address, blockNumber]});
            return toNumber(result);
        },

        getStorageAt: function (arg1, arg2, arg3) {
            return send({"method": "entropy_getStorageAt", "params": [arg1, arg2, arg3]});
        },

        getCode: function (address, blockNumber) {
            return send({"method": "entropy_getCode", "params": [address, blockNumber]});
        },

        getBlock: function (identify) {
            if (isString(identify) && identify.indexOf('0x') === 0) {
                return send({"method": "entropy_getBlockByHash", "params": [identify]});
            } else {
                return send({"method": "entropy_getBlockByNumber", "params": [identify]});
            }
        },

        getUncle: function (identify, address) {
            if (isString(identify) && identify.indexOf('0x') === 0) {
                return send({"method": "entropy_getUncleByBlockHashAndIndex", "params": [identify, address]});
            } else {
                return send({"method": "entropy_getUncleByBlockNumberAndIndex", "params": [identify, address]});
            }
        },

        getCompilers: function () {
            return send({"method": "entropy_getCompilers", "params": []});
        },

        getBlockTransactionCount: function (identify) {
            if (isString(identify) && identify.indexOf('0x') === 0) {
                return send({"method": "entropy_getBlockTransactionCountByHash", "params": [identify]});
            } else {
                return send({"method": "entropy_getBlockTransactionCountByNumber", "params": [identify]});
            }
        },

        getBlockUncleCount: function (identify) {
            if (isString(identify) && identify.indexOf('0x') === 0) {
                return send({"method": "entropy_getUncleCountByBlockHash", "params": [identify]});
            } else {
                return send({"method": "entropy_getUncleCountByBlockNumber", "params": [identify]});
            }
        },

        getTransaction: function (identify) {
            return send({"method": "entropy_getTransactionByHash", "params": [identify]});
        },

        getTransactionFromBlock: function (identify, address) {
            if (isString(identify) && identify.indexOf('0x') === 0) {
                return send({"method": "entropy_getTransactionByBlockHashAndIndex", "params": [identify, address]});
            } else {
                return send({"method": "entropy_getTransactionByBlockNumberAndIndex", "params": [identify, address]});
            }
        },

        getTransactionReceipt: function (identify) {
            return send({"method": "entropy_getTransactionReceipt", "params": [identify]});
        },

        getTransactionCount: function (identify, blockNumber) {
            return send({"method": "entropy_getTransactionCount", "params": [identify, blockNumber]});
        },

        sendRawTransaction: function (identify) {
            return send({"method": "entropy_sendRawTransaction", "params": [identify]});
        },

        sendTransaction: function (identify) {
            return send({"method": "entropy_sendTransaction", "params": [identify]});
        },

        signTransaction: function (identify) {
            return send({"method": "entropy_signTransaction", "params": [identify]});
        },

        sign: function (identify, arg2) {
            return send({"method": "entropy_sign", "params": [identify, arg2]});
        },

        call: function (identify, arg2) {
            return send({"method": "entropy_call", "params": [identify, arg2]});
        },

        submitWork: function (arg1, arg2, arg3) {
            return send({"method": "entropy_submitWork", "params": [arg1, arg2, arg3]});
        },

        getWork: function () {
            return send({"method": "entropy_getWork", "params": []});
        }
    };

    personal = {
        newAccount: function (password) {
            return send({"method": "personal_newAccount", "params": [password]});
        },

        listAccounts: function () {
            return send({"method": "personal_listAccounts", "params": []});
        },

        importRawKey: function (key1, key2) {
            return send({"method": "personal_importRawKey", "params": [key1, key2]});
        },

        sign: function (arg1, arg2, arg3) {
            return send({"method": "personal_sign", "params": [arg1, arg2, arg3]});
        },

        ecRecover: function (arg1, arg2) {
            return send({"method": "personal_ecRecover", "params": [arg1, arg2]});
        },

        unlockAccount: function (arg1, arg2, arg3) {
            return send({"method": "personal_unlockAccount", "params": [arg1, arg2, arg3]});
        },

        sendTransaction: function (arg1, arg2) {
            return send({"method": "personal_sendTransaction", "params": [arg1, arg2]});
        },

        lockAccount: function (address) {
            return send({"method": "personal_lockAccount", "params": [address]});
        },

        openWallet: function (arg1, arg2) {
            return send({"method": "personal_openWallet", "params": [arg1, arg2]});
        },

        listWallets: function () {
            return send({"method": "personal_listWallets", "params": []});
        },

        deriveAccount: function (arg1, arg2, arg3) {
            return send({"method": "personal_deriveAccount", "params": [arg1, arg2, arg3]});
        },

        signTransaction: function (arg1, arg2) {
            return send({"method": "personal_signTransaction", "params": [arg1, arg2]});
        }
    };

    miner = {
        start: function () {
            return send({"method": "miner_start", "params": []});
        },

        stop: function () {
            return send({"method": "miner_stop", "params": []});
        },

        setEntropyBase: function (coinbase) {
            return send({"method": "miner_setEntropyBase", "params": [coinbase]});
        },

        setExtra: function (extra) {
            return send({"method": "miner_setExtra", "params": [extra]});
        },

        setGasPrice: function (gasPrice) {
            return send({"method": "miner_setGasPrice", "params": [gasPrice]});
        },

        getHashrate: function () {
            return send({"method": "miner_getHashrate", "params": []});
        }
    };

    admin = {
        addPeer: function (peer) {
            return send({"method": "admin_addPeer", "params": [peer]});
        },

        removePeer: function (peer) {
            return send({"method": "admin_removePeer", "params": [peer]});
        },

        exportChain: function (chain) {
            return send({"method": "admin_exportChain", "params": [chain]});
        },

        importChain: function (chain) {
            return send({"method": "admin_importChain", "params": [chain]});
        },

        sleepBlocks: function (arg1, arg2) {
            return send({"method": "admin_sleepBlocks", "params": [arg1, arg2]});
        },

        startRPC: function (arg1, arg2, arg3, arg4) {
            return send({"method": "admin_startRPC", "params": [arg1, arg2, arg3, arg4]});
        },

        stopRPC: function () {
            return send({"method": "admin_stopRPC", "params": []});
        },

        startWS: function (arg1, arg2, arg3, arg4) {
            return send({"method": "admin_startWS", "params": [arg1, arg2, arg3, arg4]});
        },

        stopWS: function () {
            return send({"method": "admin_stopWS", "params": []});
        },

        nodeInfo: function () {
            return send({"method": "admin_nodeInfo", "params": []});
        },

        peers: function () {
            return send({"method": "admin_peers", "params": []});
        },

        datadir: function () {
            return send({"method": "admin_datadir", "params": []});
        }
    };

    net = {
        listening: function () {
            return send({"method": "net_listening", "params": []});
        },

        peerCount: function () {
            var count = send({"method": "net_peerCount", "params": []});
            count = parseInt(count, 16);
            return (isNaN(count) ? 0 : count);
        },

        version: function () {
            return send({"method": "net_version", "params": []});
        }
    };
})();
