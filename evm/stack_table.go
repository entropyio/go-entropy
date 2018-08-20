package evm

import (
	"fmt"

	"github.com/entropyio/go-entropy/config"
)

func makeStackFunc(pop, push int) stackValidationFunc {
	return func(stack *Stack) error {
		if err := stack.require(pop); err != nil {
			return err
		}

		if stack.len()+push-pop > int(config.StackLimit) {
			return fmt.Errorf("stack limit reached %d (%d)", stack.len(), config.StackLimit)
		}
		return nil
	}
}

func makeDupStackFunc(n int) stackValidationFunc {
	return makeStackFunc(n, n+1)
}

func makeSwapStackFunc(n int) stackValidationFunc {
	return makeStackFunc(n, n)
}
