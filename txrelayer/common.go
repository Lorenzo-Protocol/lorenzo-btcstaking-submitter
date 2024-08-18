package txrelayer

const (
	LorenzoBtcStakingNotConfirmedErrorMessage = "not k-deep"
	LorenzoBtcStakingDuplicateTxErrorMessage  = "duplicate btc transaction"
	LorenzoTimeoutErrorMessage                = "context deadline exceeded"
	LorenzoBtcHeaderNotFoundErrorMessage      = "btc block header not found"
	PostFailedMessage                         = "post failed"

	SequenceMismatch          = "account sequence mismatch"
	BNBBTCBStakingDuplication = "duplicate event"
)

type ITxRelayer interface {
	Start()
	Stop()
	WaitForShutdown()
	ChainName() string
}
