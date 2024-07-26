package txrelayer

const (
	LorenzoBtcStakingNotConfirmedErrorMessage = "not k-deep"
	LorenzoBtcStakingDuplicateTxErrorMessage  = "duplicate btc transaction"
	LorenzoTimeoutErrorMessage                = "context deadline exceeded"
	LorenzoBtcHeaderNotFoundErrorMessage      = "btc block header not found"
	PostFailedMessage                         = "post failed"

	SequenceMismatch = "account sequence mismatch"
)

type ITxRelayer interface {
	Start()
	Stop()
	WaitForShutdown()
	ChainName() string
}
