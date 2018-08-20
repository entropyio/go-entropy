package blockchain

import (
	"github.com/entropyio/go-entropy/blockchain/model"
	"github.com/entropyio/go-entropy/blockchain/state"
	"github.com/entropyio/go-entropy/common"
	"github.com/entropyio/go-entropy/common/crypto"
	"github.com/entropyio/go-entropy/config"
	"github.com/entropyio/go-entropy/consensus"
	"github.com/entropyio/go-entropy/evm"
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
func (p *StateProcessor) Process(block *model.Block, statedb *state.StateDB, cfg evm.Config) (model.Receipts, []*model.Log, uint64, error) {
	transactionLog.Debugf("Process input: blockNum=%d, gasLimit=%d, gasUsed=%d, td=%d, txs=%d", block.Number(), block.GasLimit(), block.GasUsed(), block.Difficulty(), block.Transactions().Len())
	var (
		receipts model.Receipts
		usedGas  = new(uint64)
		header   = block.Header()
		allLogs  []*model.Log
		gp       = new(GasPool).AddGas(block.GasLimit())
	)

	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		statedb.Prepare(tx.Hash(), block.Hash(), i)
		receipt, _, err := ApplyTransaction(p.config, p.bc, nil, gp, statedb, header, tx, usedGas, cfg)
		if err != nil {
			return nil, nil, 0, err
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	p.engine.Finalize(p.bc, header, statedb, block.Transactions(), block.Uncles(), receipts, nil)

	transactionLog.Debugf("Process output: blockNum=%d, gasLimit=%d, gasUsed=%d, td=%d, receipts=%d, allLogs=%d", block.Number(), block.GasLimit(), block.GasUsed(), block.Difficulty(), receipts.Len(), len(allLogs))
	return receipts, allLogs, *usedGas, nil
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func ApplyTransaction(config *config.ChainConfig, bc ChainContext, author *common.Address, gp *GasPool, statedb *state.StateDB, header *model.Header, tx *model.Transaction, usedGas *uint64, cfg evm.Config) (*model.Receipt, uint64, error) {
	transactionLog.Warningf("ApplyTransaction input: header number=%d, td=%d; remainGas=%d, usedGas=%d; author=%X",
		header.Number, header.Difficulty, gp.Gas(), usedGas, author)

	msg, err := tx.AsMessage(model.MakeSigner(config, header.Number))
	if err != nil {
		return nil, 0, err
	}
	// Create a new context to be used in the EVM environment
	context := NewEVMContext(msg, header, bc, author)
	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	vmenv := evm.NewEVM(context, statedb, config, cfg)
	// Apply the transaction to the current state (included in the env)
	_, gas, failed, err := ApplyMessage(vmenv, msg, gp)
	if err != nil {
		return nil, 0, err
	}
	// Update the state with pending changes
	var root []byte
	if config.IsHomestead(header.Number) {
		statedb.Finalise(true)
	} else {
		// TODO: use finalise or IntermediateRoot ?
		//root = statedb.IntermediateRoot(config.IsEIP158(header.Number)).Bytes()
	}
	*usedGas += gas

	// Create a new receipt for the transaction, storing the intermediate root and gas used by the tx
	// based on the eip phase, we're passing wether the root touch-delete account.
	receipt := model.NewReceipt(root, failed, *usedGas)
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = gas
	// if the transaction created a contract, store the creation address in the receipt.
	if msg.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(vmenv.Context.Origin, tx.Nonce())
	}
	// Set the receipt logs and create a bloom for filtering
	receipt.Logs = statedb.GetLogs(tx.Hash())
	receipt.Bloom = model.CreateBloom(model.Receipts{receipt})

	transactionLog.Warningf("ApplyTransaction output: remainGas=%d, usedGas=%d; ", gp.Gas(), gas)
	return receipt, gas, err
}
