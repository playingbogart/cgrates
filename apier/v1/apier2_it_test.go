// +build integration

/*
Real-time Online/Offline Charging System (OCS) for Telecom & ISP environments
Copyright (C) ITsysCOM GmbH

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package v1

import (
	"net/rpc"
	"net/rpc/jsonrpc"
	"path"
	"testing"
	"time"

	"github.com/cgrates/cgrates/config"
	"github.com/cgrates/cgrates/dispatchers"
	"github.com/cgrates/cgrates/engine"
	"github.com/cgrates/cgrates/utils"
)

var (
	apierCfgPath   string
	apierCfg       *config.CGRConfig
	apierRPC       *rpc.Client
	apierConfigDIR string //run tests for specific configuration
)

var sTestsAPIer = []func(t *testing.T){
	testAPIerInitCfg,
	testAPIerInitDataDb,
	testAPIerResetStorDb,
	testAPIerStartEngine,
	testAPIerRPCConn,
	testAPIerLoadFromFolder,
	testAPIerDeleteTPFromFolder,
	testAPIerAfterDelete,
	testAPIerLoadFromFolder,
	testAPIerGetRatingPlanCost,
	testAPIerGetRatingPlanCost2,
	testAPIerGetRatingPlanCost3,
	testAPIerKillEngine,
}

//Test start here
func TestAPIerIT(t *testing.T) {
	apierConfigDIR = "tutmysql"
	for _, stest := range sTestsAPIer {
		t.Run(apierConfigDIR, stest)
	}
}

func testAPIerInitCfg(t *testing.T) {
	var err error
	apierCfgPath = path.Join(costDataDir, "conf", "samples", apierConfigDIR)
	apierCfg, err = config.NewCGRConfigFromPath(apierCfgPath)
	if err != nil {
		t.Error(err)
	}
	apierCfg.DataFolderPath = costDataDir // Share DataFolderPath through config towards StoreDb for Flush()
	config.SetCgrConfig(apierCfg)
}

func testAPIerInitDataDb(t *testing.T) {
	if err := engine.InitDataDb(apierCfg); err != nil {
		t.Fatal(err)
	}
}

// Wipe out the cdr database
func testAPIerResetStorDb(t *testing.T) {
	if err := engine.InitStorDb(apierCfg); err != nil {
		t.Fatal(err)
	}
}

// Start CGR Engine
func testAPIerStartEngine(t *testing.T) {
	if _, err := engine.StopStartEngine(apierCfgPath, *waitRater); err != nil {
		t.Fatal(err)
	}
}

// Connect rpc client to rater
func testAPIerRPCConn(t *testing.T) {
	var err error
	apierRPC, err = jsonrpc.Dial("tcp", apierCfg.ListenCfg().RPCJSONListen) // We connect over JSON so we can also troubleshoot if needed
	if err != nil {
		t.Fatal(err)
	}
}

func testAPIerLoadFromFolder(t *testing.T) {
	var reply string
	attrs := &utils.AttrLoadTpFromFolder{FolderPath: path.Join(*dataDir, "tariffplans", "tutorial")}
	if err := apierRPC.Call("ApierV1.LoadTariffPlanFromFolder", attrs, &reply); err != nil {
		t.Error(err)
	}
	time.Sleep(500 * time.Millisecond)
}

func testAPIerDeleteTPFromFolder(t *testing.T) {
	var reply string
	attrs := &utils.AttrLoadTpFromFolder{FolderPath: path.Join(*dataDir, "tariffplans", "tutorial")}
	if err := apierRPC.Call("ApierV1.DeleteTPFromFolder", attrs, &reply); err != nil {
		t.Error(err)
	}
	time.Sleep(500 * time.Millisecond)
}

func testAPIerAfterDelete(t *testing.T) {
	var reply *engine.AttributeProfile
	if err := apierRPC.Call("ApierV1.GetAttributeProfile",
		&utils.TenantID{Tenant: "cgrates.org", ID: "ApierTest"}, &reply); err == nil ||
		err.Error() != utils.ErrNotFound.Error() {
		t.Fatal(err)
	}
	var replyTh *engine.ThresholdProfile
	if err := apierRPC.Call("ApierV1.GetThresholdProfile",
		&utils.TenantID{Tenant: "cgrates.org", ID: "THD_Test"}, &replyTh); err == nil ||
		err.Error() != utils.ErrNotFound.Error() {
		t.Error(err)
	}
}

func testAPIerGetRatingPlanCost(t *testing.T) {
	arg := &utils.RatingPlanCostArg{
		Destination:   "1002",
		RatingPlanIDs: []string{"RP_1001", "RP_1002"},
		SetupTime:     utils.META_NOW,
		Usage:         "1h",
	}
	var reply dispatchers.RatingPlanCost
	if err := apierRPC.Call(utils.RALsV1GetRatingPlansCost, arg, &reply); err != nil {
		t.Error(err)
	} else if reply.RatingPlanID != "RP_1001" {
		t.Error("Unexpected RatingPlanID: ", reply.RatingPlanID)
	} else if *reply.EventCost.Cost != 6.5118 {
		t.Error("Unexpected Cost: ", *reply.EventCost.Cost)
	} else if *reply.EventCost.Usage != time.Duration(time.Hour) {
		t.Error("Unexpected Usage: ", *reply.EventCost.Usage)
	}
}

// we need to discuss about this case
// because 1003 have the following DestinationRate
// DR_1003_MAXCOST_DISC,DST_1003,RT_1CNT_PER_SEC,*up,4,0.12,*disconnect
func testAPIerGetRatingPlanCost2(t *testing.T) {
	arg := &utils.RatingPlanCostArg{
		Destination:   "1003",
		RatingPlanIDs: []string{"RP_1001", "RP_1002"},
		SetupTime:     utils.META_NOW,
		Usage:         "1h",
	}
	var reply dispatchers.RatingPlanCost
	if err := apierRPC.Call(utils.RALsV1GetRatingPlansCost, arg, &reply); err != nil {
		t.Error(err)
	} else if reply.RatingPlanID != "RP_1001" {
		t.Error("Unexpected RatingPlanID: ", reply.RatingPlanID)
	} else if *reply.EventCost.Cost != 36 {
		t.Error("Unexpected Cost: ", *reply.EventCost.Cost)
	} else if *reply.EventCost.Usage != time.Duration(time.Hour) {
		t.Error("Unexpected Usage: ", *reply.EventCost.Usage)
	}
}

func testAPIerGetRatingPlanCost3(t *testing.T) {
	arg := &utils.RatingPlanCostArg{
		Destination:   "1001",
		RatingPlanIDs: []string{"RP_1001", "RP_1002"},
		SetupTime:     utils.META_NOW,
		Usage:         "1h",
	}
	var reply dispatchers.RatingPlanCost
	if err := apierRPC.Call(utils.RALsV1GetRatingPlansCost, arg, &reply); err != nil {
		t.Error(err)
	} else if reply.RatingPlanID != "RP_1002" {
		t.Error("Unexpected RatingPlanID: ", reply.RatingPlanID)
	} else if *reply.EventCost.Cost != 36 {
		t.Error("Unexpected Cost: ", *reply.EventCost.Cost)
	} else if *reply.EventCost.Usage != time.Duration(time.Hour) {
		t.Error("Unexpected Usage: ", *reply.EventCost.Usage)
	}
}

func testAPIerKillEngine(t *testing.T) {
	if err := engine.KillEngine(*waitRater); err != nil {
		t.Error(err)
	}
}