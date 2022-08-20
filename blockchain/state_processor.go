package blockchain

import (
	"fmt"
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/evm"
	"math/big"
)

// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type StateProcessor struct {
	config *config.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}

// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(config *config.ChainConfig, bc *BlockChain, engine consensus.Engine) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// Process processes the state changes according to the Entropy rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) Process(block *model.Block, stateDB *state.StateDB, cfg evm.Config) (model.Receipts, []*model.Log, uint64, error) {
	txLog.Debugf("Process input: blockNum=%d, gasLimit=%d, gasUsed=%d, td=%d, txs=%d", block.Number(), block.GasLimit(), block.GasUsed(), block.Difficulty(), block.Transactions().Len())
	var (
		receipts    model.Receipts
		usedGas     = new(uint64)
		header      = block.Header()
		blockHash   = block.Hash()
		blockNumber = block.Number()
		allLogs     []*model.Log
		gp          = new(GasPool).AddGas(block.GasLimit())
	)
	// Mutate the block and state according to any hard-fork specs
	//if p.config.DAOForkSupport && p.config.DAOForkBlock != nil && p.config.DAOForkBlock.Cmp(block.Number()) == 0 {
	//	misc.ApplyDAOHardFork(statedb)
	//}
	blockContext := NewEVMBlockContext(header, p.bc, nil)
	vmenv := evm.NewEVM(blockContext, evm.TxContext{}, stateDB, p.config, cfg)
	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		msg, err := tx.AsMessage(model.MakeSigner(p.config, header.Number), header.BaseFee)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}
		stateDB.Prepare(tx.Hash(), i)
		receipt, err := applyTransaction(msg, p.config, p.bc, nil, gp, stateDB, blockNumber, blockHash, tx, usedGas, vmenv)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	p.engine.Finalize(p.bc, header, stateDB, block.Transactions(), block.Uncles())

	txLog.Debugf("Process output: blockNum=%d, gasLimit=%d, gasUsed=%d, td=%d, receipts=%d, allLogs=%d", block.Number(), block.GasLimit(), block.GasUsed(), block.Difficulty(), len(receipts), len(allLogs))
	return receipts, allLogs, *usedGas, nil
}

func applyTransaction(msg model.Message, conf *config.ChainConfig, bc ChainContext, author *common.Address, gp *GasPool, statedb *state.StateDB, blockNumber *big.Int, blockHash common.Hash, tx *model.Transaction, usedGas *uint64, vm *evm.EVM) (*model.Receipt, error) {
	// Create a new context to be used in the EVM environment.
	txContext := NewEVMTxContext(msg)
	vm.Reset(txContext, statedb)

	// Apply the transaction to the current state (included in the env).
	result, err := ApplyMessage(vm, msg, gp)
	if err != nil {
		return nil, err
	}

	// Update the state with pending changes.
	var root = statedb.IntermediateRoot(false).Bytes()
	*usedGas += result.UsedGas

	// Create a new receipt for the transaction, storing the intermediate root and gas used
	// by the tx.
	receipt := &model.Receipt{Type: tx.Type(), PostState: root, CumulativeGasUsed: *usedGas}
	if result.Failed() {
		receipt.Status = model.ReceiptStatusFailed
	} else {
		receipt.Status = model.ReceiptStatusSuccessful
	}
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = result.UsedGas

	// If the transaction created a contract, store the creation address in the receipt.
	if msg.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(vm.TxContext.Origin, tx.Nonce())
	}

	// Set the receipt logs and create the bloom filter.
	receipt.Logs = statedb.GetLogs(tx.Hash(), blockHash)
	receipt.Bloom = model.CreateBloom(model.Receipts{receipt})
	receipt.BlockHash = blockHash
	receipt.BlockNumber = blockNumber
	receipt.TransactionIndex = uint(statedb.TxIndex())
	return receipt, err
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func ApplyTransaction(config *config.ChainConfig, bc ChainContext, author *common.Address, gp *GasPool, statedb *state.StateDB, header *model.Header, tx *model.Transaction, usedGas *uint64, cfg evm.Config) (*model.Receipt, error) {
	msg, err := tx.AsMessage(model.MakeSigner(config, header.Number), header.BaseFee)
	if err != nil {
		return nil, err
	}
	// Create a new context to be used in the EVM environment
	blockContext := NewEVMBlockContext(header, bc, author)
	vmenv := evm.NewEVM(blockContext, evm.TxContext{}, statedb, config, cfg)
	return applyTransaction(msg, config, bc, author, gp, statedb, header.Number, header.Hash(), tx, usedGas, vmenv)
}
