package txrelayer

type ITxRelayer interface {
	Start()
	Stop()
	WaitForShutdown()
	ChainName() string
}

type IExpectedBNBLorenzoClient interface {
}
