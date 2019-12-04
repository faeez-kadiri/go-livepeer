package eth

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/livepeer/go-livepeer/eth/contracts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromPerc_DefaultDenominator(t *testing.T) {
	assert.Equal(t, big.NewInt(1000000), FromPerc(100.0))

	assert.Equal(t, big.NewInt(500000), FromPerc(50.0))

	assert.Equal(t, big.NewInt(0), FromPerc(0.0))
}

func TestFromPercOfUint256_Given100Percent_ResultWithinEpsilon(t *testing.T) {
	actual := FromPercOfUint256(100.0)

	diff := new(big.Int).Sub(maxUint256, actual)
	assert.True(t, diff.Int64() < 100)
}
func TestFromPercOfUint256_Given50Percent_ResultWithinEpsilon(t *testing.T) {
	half := new(big.Int).Div(maxUint256, big.NewInt(2))

	actual := FromPercOfUint256(50.0)

	diff := new(big.Int).Sub(half, actual)
	assert.True(t, diff.Int64() < 100)
}

func TestFromPercOfUint256_Given0Percent_ReturnsZero(t *testing.T) {
	assert.Equal(t, int64(0), FromPercOfUint256(0.0).Int64())
}

func TestDecodeTxParams(t *testing.T) {
	// TicketBroker.fundDepositAndReserve(_depositAmount: 10000000000000000, _reserveAmount: 10000000000000000)
	data := ethcommon.Hex2Bytes("511f4073000000000000000000000000000000000000000000000000002386f26fc10000000000000000000000000000000000000000000000000000002386f26fc10000")
	txParams := make(map[string]interface{})
	depositAmount, _ := new(big.Int).SetString("10000000000000000", 10)
	reserveAmount, _ := new(big.Int).SetString("10000000000000000", 10)
	ticketBroker, err := abi.JSON(strings.NewReader(contracts.TicketBrokerABI))
	require.NoError(t, err)

	txParams, err = decodeTxParams(ticketBroker, txParams, data)
	assert.Equal(t, txParams["_depositAmount"], depositAmount)
	assert.Equal(t, txParams["_reserveAmount"], reserveAmount)
}
