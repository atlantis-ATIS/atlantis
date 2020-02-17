// Copyright 2016 The go-athereum Authors
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

package athclient

import "github.com/athereum/go-athereum"

// Verify that Client implements theatlantis interfaces.
var (
	_ =atlantis.ChainReader(&Client{})
	_ =atlantis.TransactionReader(&Client{})
	_ =atlantis.ChainStateReader(&Client{})
	_ =atlantis.ChainSyncReader(&Client{})
	_ =atlantis.ContractCaller(&Client{})
	_ =atlantis.GasEstimator(&Client{})
	_ =atlantis.GasPricer(&Client{})
	_ =atlantis.LogFilterer(&Client{})
	_ =atlantis.PendingStateReader(&Client{})
	// _ =atlantis.PendingStateEventer(&Client{})
	_ =atlantis.PendingContractCaller(&Client{})
)
