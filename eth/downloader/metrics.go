// Copyright 2015 The go-athereum Authors
// This file is part of the go-athereum library.
//
// The go-athereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-athereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-athereum library. If not, see <http://www.gnu.org/licenses/>.

// Contains the metrics collected by the downloader.

package downloader

import (
	"github.com/athereum/go-athereum/metrics"
)

var (
	headerInMeter      = metrics.NewRegisteredMeter("ath/downloader/headers/in", nil)
	headerReqTimer     = metrics.NewRegisteredTimer("ath/downloader/headers/req", nil)
	headerDropMeter    = metrics.NewRegisteredMeter("ath/downloader/headers/drop", nil)
	headerTimeoutMeter = metrics.NewRegisteredMeter("ath/downloader/headers/timeout", nil)

	bodyInMeter      = metrics.NewRegisteredMeter("ath/downloader/bodies/in", nil)
	bodyReqTimer     = metrics.NewRegisteredTimer("ath/downloader/bodies/req", nil)
	bodyDropMeter    = metrics.NewRegisteredMeter("ath/downloader/bodies/drop", nil)
	bodyTimeoutMeter = metrics.NewRegisteredMeter("ath/downloader/bodies/timeout", nil)

	receiptInMeter      = metrics.NewRegisteredMeter("ath/downloader/receipts/in", nil)
	receiptReqTimer     = metrics.NewRegisteredTimer("ath/downloader/receipts/req", nil)
	receiptDropMeter    = metrics.NewRegisteredMeter("ath/downloader/receipts/drop", nil)
	receiptTimeoutMeter = metrics.NewRegisteredMeter("ath/downloader/receipts/timeout", nil)

	stateInMeter   = metrics.NewRegisteredMeter("ath/downloader/states/in", nil)
	stateDropMeter = metrics.NewRegisteredMeter("ath/downloader/states/drop", nil)
)
