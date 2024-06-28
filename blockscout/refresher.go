package blockscout

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"net/http"
	"strconv"
	"time"

	"github.com/Lorenzo-Protocol/lorenzo-btcstaking-submitter/config"
)

type Refresher struct {
	blockscoutApi     string
	lorenzoAppApi     string
	nextRefreshHeight uint64

	logger *zap.Logger
}

func NewRefresher(startHeight uint64, blockscoutApi string, lorenzoAppApi string) (*Refresher, error) {
	logger, err := config.NewRootLogger("json", true)
	if err != nil {
		return nil, err
	}

	return &Refresher{
		blockscoutApi:     blockscoutApi,
		lorenzoAppApi:     lorenzoAppApi,
		nextRefreshHeight: startHeight,
		logger:            logger,
	}, nil
}

func (r *Refresher) Start() error {
	networkErrorSleepTime := time.Millisecond * 300
	lorenzoBlockHeightSleepTime := time.Second * 5
	batchNum := uint64(1000)
	for {
		blockscoutHeight, err := r.getBockscoutCurrentHeight()
		if err != nil {
			time.Sleep(networkErrorSleepTime)
			continue
		}
		startHeight := r.nextRefreshHeight
		if blockscoutHeight < startHeight {
			r.logger.Warn("blockscout height is less than start height",
				zap.Uint64("blockscoutHeight", blockscoutHeight), zap.Uint64("startHeight", startHeight))
			time.Sleep(lorenzoBlockHeightSleepTime)
			continue
		}

		endHeight := r.nextRefreshHeight + batchNum
		if endHeight > blockscoutHeight+1 {
			endHeight = blockscoutHeight + 1
		}

		eventScanCursor, err := r.getLorenzoEventScanCursor()
		if err != nil {
			r.logger.Warn("get EventScanCursor failed", zap.Error(err))
			time.Sleep(networkErrorSleepTime)
			continue
		}
		if eventScanCursor < startHeight {
			r.logger.Warn("event scan cursor is less than start height",
				zap.Uint64("eventScanCursor", eventScanCursor), zap.Uint64("startHeight", startHeight))
			time.Sleep(lorenzoBlockHeightSleepTime)
			continue
		}
		if eventScanCursor+1 < endHeight {
			endHeight = eventScanCursor + 1
		}
		events, err := r.getLorenzoBurnOrEventListByHeightRange(r.lorenzoAppApi, startHeight, endHeight)
		if err != nil {
			r.logger.Warn("get events failed", zap.Error(err))
			time.Sleep(networkErrorSleepTime)
			continue
		}

		if len(events) == 0 {
			r.logger.Warn("no events", zap.Uint64("fromHeight", r.nextRefreshHeight), zap.Uint64("toHeight", endHeight))
			r.nextRefreshHeight = endHeight
			continue
		}

		i := 0
		for i < len(events) {
			event := events[i]
			if err := r.refreshBlockscoutBalance(event.LorenzoAddr, event.LorenzoBlockHeight); err != nil {
				r.logger.Warn("refresh failed", zap.Error(err))
				time.Sleep(networkErrorSleepTime)
				continue
			}

			r.logger.Info("account balance refreshed", zap.String("address", event.LorenzoAddr),
				zap.Uint64("height", event.LorenzoBlockHeight))
			i++
		}

		r.logger.Info("finish refresh", zap.Int("total", len(events)), zap.Uint64("fromHeight", r.nextRefreshHeight), zap.Uint64("toHeight", endHeight))
		r.nextRefreshHeight = endHeight
	}
}

func (r *Refresher) getBockscoutCurrentHeight() (uint64, error) {
	var respData struct {
		Jsonrpc string `json:"jsonrpc"`
		Result  string `json:"result"`
		Id      int    `json:"id"`
	}
	url := fmt.Sprintf("%s?module=block&action=eth_block_number", r.blockscoutApi)
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	if err := checkBlockstreamResponse(resp); err != nil {
		return 0, err
	}

	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return 0, err
	}

	height, err := strconv.ParseUint(respData.Result, 0, 64)
	if err != nil {
		return 0, err
	}

	return height, nil
}

func (r *Refresher) checkOrInsertAccountToBlockscout(addr string) error {
	url := fmt.Sprintf("%s/v2/search/check-redirect?q=%s", r.blockscoutApi, addr)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	var respData struct {
		Parameter string `json:"parameter"`
		Redirect  bool   `json:"redirect"`
		Type      string `json:"type"`
	}
	if err := checkBlockstreamResponse(resp); err != nil {
		return err
	}

	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return err
	}
	if respData.Type != "address" {
		return fmt.Errorf("invalid type: %s", respData.Type)
	}

	return nil
}

func (r *Refresher) refreshBlockscoutBalance(addr string, eventHeight uint64) error {
	if err := r.checkOrInsertAccountToBlockscout(addr); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/v2/addresses/%s/refresh", r.blockscoutApi, addr)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	if err := checkBlockstreamResponse(resp); err != nil {
		return err
	}

	var respData struct {
		Result string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return err
	}
	if respData.Result != "executed update" {
		return errors.New("update failed")
	}

	return nil
}

func (r *Refresher) getLorenzoBurnOrEventListByHeightRange(lorenzoAppApi string, startHeight, endHeight uint64) ([]LorenzoBurnAndMintEvent, error) {
	responseData := struct {
		Code int `json:"code"`
		Data struct {
			List  []LorenzoBurnAndMintEvent `json:"list"`
			Total int                       `json:"total"`
		} `json:"data"`
	}{}
	resp, err := http.Get(fmt.Sprintf("%s/list?block_height_range=%d-%d", lorenzoAppApi, startHeight, endHeight))
	if err != nil {
		return nil, err
	}

	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return nil, err
	}

	return responseData.Data.List, nil
}
func (r *Refresher) getLorenzoEventScanCursor() (uint64, error) {
	resp, err := http.Get(fmt.Sprintf("%s/block_scan_cursor", r.lorenzoAppApi))
	if err != nil {
		return 0, err
	}
	if err := checkBlockstreamResponse(resp); err != nil {
		return 0, err
	}

	var respData struct {
		Code int    `json:"code"`
		Data uint64 `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return 0, err
	}

	return respData.Data, nil
}

func checkBlockstreamResponse(resp *http.Response) error {
	if resp.StatusCode != http.StatusOK {
		var errorBuf bytes.Buffer
		_, _ = errorBuf.ReadFrom(resp.Body)

		return errors.New(errorBuf.String())
	}

	return nil
}
