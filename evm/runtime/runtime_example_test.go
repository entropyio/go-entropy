package runtime

import (
	"fmt"
	"github.com/entropyio/go-entropy/common"
)

func ExampleExecute() {
	ret, _, err := Execute(common.Hex2Bytes("6060604052600a8060106000396000f360606040526008565b00"), nil, nil)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(ret)
	// Output:
	// [96 96 96 64 82 96 8 86 91 0]
}
