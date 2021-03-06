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
package v2

import (
	"errors"
	"flag"
	"fmt"
	"net/rpc"
	"net/rpc/jsonrpc"
	"path"
	"reflect"
	"strconv"
	"testing"
	"time"

	v1 "github.com/cgrates/cgrates/apier/v1"
	"github.com/cgrates/cgrates/config"
	"github.com/cgrates/cgrates/engine"
	"github.com/cgrates/cgrates/utils"
)

var (
	dataDir      = flag.String("data_dir", "/usr/share/cgrates", "CGR data dir path here")
	waitRater    = flag.Int("wait_rater", 1500, "Number of miliseconds to wait for rater to start and cache")
	encoding     = flag.String("rpc", utils.MetaJSON, "what encoding whould be uused for rpc comunication")
	apierCfgPath string
	apierCfg     *config.CGRConfig
	apierRPC     *rpc.Client
	dm           *engine.DataManager // share db connection here so we can check data we set through APIs
)

func newRPCClient(cfg *config.ListenCfg) (c *rpc.Client, err error) {
	switch *encoding {
	case utils.MetaJSON:
		return jsonrpc.Dial(utils.TCP, cfg.RPCJSONListen)
	case utils.MetaGOB:
		return rpc.Dial(utils.TCP, cfg.RPCGOBListen)
	default:
		return nil, errors.New("UNSUPPORTED_RPC")
	}
}
func TestApierV2itLoadConfig(t *testing.T) {
	apierCfgPath = path.Join(*dataDir, "conf", "samples", "tutmysql")
	if apierCfg, err = config.NewCGRConfigFromPath(apierCfgPath); err != nil {
		t.Error(err)
	}
}

// Remove data in both rating and accounting db
func TestApierV2itResetDataDb(t *testing.T) {
	if err := engine.InitDataDb(apierCfg); err != nil {
		t.Fatal(err)
	}
}

// Wipe out the cdr database
func TestApierV2itResetStorDb(t *testing.T) {
	if err := engine.InitStorDb(apierCfg); err != nil {
		t.Fatal(err)
	}
}

func TestApierV2itConnectDataDB(t *testing.T) {
	rdsDb, _ := strconv.Atoi(apierCfg.DataDbCfg().DataDbName)
	if rdsITdb, err := engine.NewRedisStorage(
		fmt.Sprintf("%s:%s", apierCfg.DataDbCfg().DataDbHost, apierCfg.DataDbCfg().DataDbPort),
		rdsDb, apierCfg.DataDbCfg().DataDbPass, apierCfg.GeneralCfg().DBDataEncoding,
		utils.REDIS_MAX_CONNS, ""); err != nil {
		t.Fatal("Could not connect to Redis", err.Error())
	} else {
		dm = engine.NewDataManager(rdsITdb, config.CgrConfig().CacheCfg(), nil, nil)
	}
}

// Start CGR Engine
func TestApierV2itStartEngine(t *testing.T) {
	if _, err := engine.StopStartEngine(apierCfgPath, 200); err != nil { // Mongo requires more time to start
		t.Fatal(err)
	}
}

// Connect rpc client to rater
func TestApierV2itRpcConn(t *testing.T) {
	apierRPC, err = newRPCClient(apierCfg.ListenCfg()) // We connect over JSON so we can also troubleshoot if needed
	if err != nil {
		t.Fatal(err)
	}
}

func TestApierV2itAddBalance(t *testing.T) {
	attrs := &utils.AttrSetBalance{
		Tenant:      "cgrates.org",
		Account:     "dan",
		BalanceType: utils.MONETARY,
		Balance: map[string]interface{}{
			utils.ID:     utils.MetaDefault,
			utils.Value:  5.0,
			utils.Weight: 10.0,
		},
	}
	var reply string
	if err := apierRPC.Call(utils.ApierV2SetBalance, attrs, &reply); err != nil {
		t.Fatal(err)
	}
	var acnt engine.Account
	if err := apierRPC.Call(utils.ApierV2GetAccount, &utils.AttrGetAccount{Tenant: "cgrates.org", Account: "dan"}, &acnt); err != nil {
		t.Error(err)
	} else if acnt.BalanceMap[utils.MONETARY][0].Value != 5.0 {
		t.Errorf("Unexpected balance received: %+v", acnt.BalanceMap[utils.MONETARY][0])
	}
}

func TestApierV2itSetAction(t *testing.T) {
	attrs := utils.AttrSetActions{ActionsId: "DISABLE_ACCOUNT", Actions: []*utils.TPAction{
		{Identifier: utils.DISABLE_ACCOUNT, Weight: 10.0},
	}}
	var reply string
	if err := apierRPC.Call(utils.ApierV2SetActions, attrs, &reply); err != nil {
		t.Error(err)
	}
	var acts map[string]engine.Actions
	if err := apierRPC.Call(utils.ApierV2GetActions, AttrGetActions{ActionIDs: []string{attrs.ActionsId}}, &acts); err != nil {
		t.Error(err)
	} else if len(acts) != 1 {
		t.Errorf("Received actions: %+v", acts)
	}
}

func TestApierV2itSetAccountActionTriggers(t *testing.T) {
	attrs := v1.AttrSetAccountActionTriggers{
		Tenant:  "cgrates.org",
		Account: "dan",
		AttrSetActionTrigger: v1.AttrSetActionTrigger{
			GroupID: "MONITOR_MAX_BALANCE",
			ActionTrigger: map[string]interface{}{
				utils.ThresholdType:  utils.TRIGGER_MAX_BALANCE,
				utils.ThresholdValue: 50,
				utils.BalanceType:    utils.MONETARY,
				utils.ActionsID:      "DISABLE_ACCOUNT",
			},
		},
	}
	var reply string
	if err := apierRPC.Call(utils.ApierV2SetAccountActionTriggers, attrs, &reply); err != nil {
		t.Error(err)
	}
	var ats engine.ActionTriggers
	if err := apierRPC.Call(utils.ApierV2GetAccountActionTriggers, utils.TenantAccount{Tenant: "cgrates.org", Account: "dan"}, &ats); err != nil {
		t.Error(err)
	} else if len(ats) != 1 || ats[0].ID != attrs.GroupID || ats[0].ThresholdValue != 50.0 {
		t.Errorf("Received: %+v", ats)
	}
	attrs.ActionTrigger[utils.ThresholdValue] = 55 // Change the threshold
	if err := apierRPC.Call(utils.ApierV2SetAccountActionTriggers, attrs, &reply); err != nil {
		t.Error(err)
	}
	if err := apierRPC.Call(utils.ApierV2GetAccountActionTriggers, utils.TenantAccount{Tenant: "cgrates.org", Account: "dan"}, &ats); err != nil {
		t.Error(err)
	} else if len(ats) != 1 || ats[0].ID != attrs.GroupID || ats[0].ThresholdValue != 55.0 {
		t.Errorf("Received: %+v", ats)
	}
}

func TestApierV2itFraudMitigation(t *testing.T) {
	attrs := &utils.AttrSetBalance{
		Tenant:      "cgrates.org",
		Account:     "dan",
		BalanceType: utils.MONETARY,
		Balance: map[string]interface{}{
			utils.ID:     utils.MetaDefault,
			utils.Value:  60.0,
			utils.Weight: 10.0,
		},
	}
	var reply string
	if err := apierRPC.Call(utils.ApierV2SetBalance, attrs, &reply); err != nil {
		t.Fatal(err)
	}
	var acnt engine.Account
	if err := apierRPC.Call(utils.ApierV2GetAccount, &utils.AttrGetAccount{Tenant: "cgrates.org", Account: "dan"}, &acnt); err != nil {
		t.Error(err)
	} else if len(acnt.BalanceMap) != 1 || acnt.BalanceMap[utils.MONETARY][0].Value != 60.0 {
		t.Errorf("Unexpected balance received: %+v", acnt.BalanceMap[utils.MONETARY][0])
	} else if !acnt.Disabled {
		t.Fatalf("Received account: %+v", acnt)
	}
	attrSetAcnt := AttrSetAccount{
		Tenant:  "cgrates.org",
		Account: "dan",
		ExtraOptions: map[string]bool{
			utils.Disabled: false,
		},
	}
	if err := apierRPC.Call(utils.ApierV2SetAccount, attrSetAcnt, &reply); err != nil {
		t.Fatal(err)
	}
	acnt = engine.Account{} // gob doesn't update the fields with default values
	if err := apierRPC.Call(utils.ApierV2GetAccount, &utils.AttrGetAccount{Tenant: "cgrates.org", Account: "dan"}, &acnt); err != nil {
		t.Error(err)
	} else if len(acnt.BalanceMap) != 1 || acnt.BalanceMap[utils.MONETARY][0].Value != 60.0 {
		t.Errorf("Unexpected balance received: %+v", acnt.BalanceMap[utils.MONETARY][0])
	} else if acnt.Disabled {
		t.Fatalf("Received account: %+v", acnt)
	}
}

func TestApierV2itSetAccountWithAP(t *testing.T) {
	argActs1 := utils.AttrSetActions{ActionsId: "TestApierV2itSetAccountWithAP_ACT_1",
		Actions: []*utils.TPAction{
			{Identifier: utils.TOPUP_RESET,
				BalanceType: utils.MONETARY, Units: "5.0", Weight: 20.0},
		}}
	var reply string
	if err := apierRPC.Call(utils.ApierV2SetActions, argActs1, &reply); err != nil {
		t.Error(err)
	}
	tNow := time.Now().Add(time.Duration(time.Minute))
	argAP1 := &v1.AttrSetActionPlan{Id: "TestApierV2itSetAccountWithAP_AP_1",
		ActionPlan: []*v1.AttrActionPlan{
			{ActionsId: argActs1.ActionsId,
				Time:   fmt.Sprintf("%v:%v:%v", tNow.Hour(), tNow.Minute(), tNow.Second()), // 10:4:12
				Weight: 20.0}}}
	if _, err := dm.GetActionPlan(argAP1.Id, true, utils.NonTransactional); err == nil || err != utils.ErrNotFound {
		t.Error(err)
	}
	if err := apierRPC.Call(utils.ApierV1SetActionPlan, argAP1, &reply); err != nil {
		t.Error("Got error on ApierV1.SetActionPlan: ", err.Error())
	} else if reply != utils.OK {
		t.Errorf("Calling ApierV1.SetActionPlan received: %s", reply)
	}
	argSetAcnt1 := AttrSetAccount{
		Tenant:        "cgrates.org",
		Account:       "TestApierV2itSetAccountWithAP1",
		ActionPlanIDs: []string{argAP1.Id},
	}
	acntID := utils.ConcatenatedKey(argSetAcnt1.Tenant, argSetAcnt1.Account)
	if _, err := dm.GetAccountActionPlans(acntID, true, utils.NonTransactional); err == nil || err != utils.ErrNotFound {
		t.Error(err)
	}
	if err := apierRPC.Call(utils.ApierV2SetAccount, argSetAcnt1, &reply); err != nil {
		t.Fatal(err)
	}
	if ap, err := dm.GetActionPlan(argAP1.Id, true, utils.NonTransactional); err != nil {
		t.Error(err)
	} else if _, hasIt := ap.AccountIDs[acntID]; !hasIt {
		t.Errorf("ActionPlan does not contain the accountID: %+v", ap)
	}
	eAAPids := []string{argAP1.Id}
	if aapIDs, err := dm.GetAccountActionPlans(acntID, true, utils.NonTransactional); err != nil {
		t.Error(err)
	} else if !reflect.DeepEqual(eAAPids, aapIDs) {
		t.Errorf("Expecting: %+v, received: %+v", eAAPids, aapIDs)
	}
	// Set second AP so we can see the proper indexing done
	argAP2 := &v1.AttrSetActionPlan{Id: "TestApierV2itSetAccountWithAP_AP_2",
		ActionPlan: []*v1.AttrActionPlan{
			{ActionsId: argActs1.ActionsId, MonthDays: "1", Time: "00:00:00", Weight: 20.0}}}
	if _, err := dm.GetActionPlan(argAP2.Id, true, utils.NonTransactional); err == nil || err != utils.ErrNotFound {
		t.Error(err)
	}
	if err := apierRPC.Call(utils.ApierV2SetActionPlan, argAP2, &reply); err != nil {
		t.Error("Got error on ApierV2.SetActionPlan: ", err.Error())
	} else if reply != utils.OK {
		t.Errorf("Calling ApierV2.SetActionPlan received: %s", reply)
	}
	// Test adding new AP
	argSetAcnt2 := AttrSetAccount{
		Tenant:        "cgrates.org",
		Account:       "TestApierV2itSetAccountWithAP1",
		ActionPlanIDs: []string{argAP2.Id},
	}
	if err := apierRPC.Call(utils.ApierV2SetAccount, argSetAcnt2, &reply); err != nil {
		t.Fatal(err)
	}
	if ap, err := dm.GetActionPlan(argAP2.Id, true, utils.NonTransactional); err != nil {
		t.Error(err)
	} else if _, hasIt := ap.AccountIDs[acntID]; !hasIt {
		t.Errorf("ActionPlan does not contain the accountID: %+v", ap)
	}
	if ap, err := dm.GetActionPlan(argAP1.Id, true, utils.NonTransactional); err != nil {
		t.Error(err)
	} else if _, hasIt := ap.AccountIDs[acntID]; !hasIt {
		t.Errorf("ActionPlan does not contain the accountID: %+v", ap)
	}
	eAAPids = []string{argAP1.Id, argAP2.Id}
	if aapIDs, err := dm.GetAccountActionPlans(acntID, true, utils.NonTransactional); err != nil {
		t.Error(err)
	} else if !reflect.DeepEqual(eAAPids, aapIDs) {
		t.Errorf("Expecting: %+v, received: %+v", eAAPids, aapIDs)
	}
	// test remove and overwrite
	argSetAcnt2 = AttrSetAccount{
		Tenant:               "cgrates.org",
		Account:              "TestApierV2itSetAccountWithAP1",
		ActionPlanIDs:        []string{argAP2.Id},
		ActionPlansOverwrite: true,
	}
	if err := apierRPC.Call(utils.ApierV2SetAccount, argSetAcnt2, &reply); err != nil {
		t.Fatal(err)
	}
	if ap, err := dm.GetActionPlan(argAP1.Id, true, utils.NonTransactional); err != nil {
		t.Error(err)
	} else if _, hasIt := ap.AccountIDs[acntID]; hasIt {
		t.Errorf("ActionPlan does contain the accountID: %+v", ap)
	}
	if ap, err := dm.GetActionPlan(argAP2.Id, true, utils.NonTransactional); err != nil {
		t.Error(err)
	} else if _, hasIt := ap.AccountIDs[acntID]; !hasIt {
		t.Errorf("ActionPlan does not contain the accountID: %+v", ap)
	}
	eAAPids = []string{argAP2.Id}
	if aapIDs, err := dm.GetAccountActionPlans(acntID, true, utils.NonTransactional); err != nil {
		t.Error(err)
	} else if !reflect.DeepEqual(eAAPids, aapIDs) {
		t.Errorf("Expecting: %+v, received: %+v", eAAPids, aapIDs)
	}
}

func TestApierV2itSetActionWithCategory(t *testing.T) {
	var reply string
	attrsSetAccount := &utils.AttrSetAccount{Tenant: "cgrates.org", Account: "TestApierV2itSetActionWithCategory"}
	if err := apierRPC.Call(utils.ApierV1SetAccount, attrsSetAccount, &reply); err != nil {
		t.Error("Got error on ApierV1.SetAccount: ", err.Error())
	} else if reply != utils.OK {
		t.Errorf("Calling ApierV1.SetAccount received: %s", reply)
	}

	argActs1 := utils.AttrSetActions{ActionsId: "TestApierV2itSetActionWithCategory_ACT",
		Actions: []*utils.TPAction{
			{Identifier: utils.TOPUP_RESET,
				BalanceType: utils.MONETARY, Categories: "test", Units: "5.0", Weight: 20.0},
		}}

	if err := apierRPC.Call(utils.ApierV2SetActions, argActs1, &reply); err != nil {
		t.Error(err)
	}

	attrsEA := &utils.AttrExecuteAction{Tenant: attrsSetAccount.Tenant, Account: attrsSetAccount.Account, ActionsId: argActs1.ActionsId}
	if err := apierRPC.Call(utils.ApierV1ExecuteAction, attrsEA, &reply); err != nil {
		t.Error("Got error on ApierV1.ExecuteAction: ", err.Error())
	} else if reply != utils.OK {
		t.Errorf("Calling ApierV1.ExecuteAction received: %s", reply)
	}

	var acnt engine.Account
	if err := apierRPC.Call(utils.ApierV2GetAccount, &utils.AttrGetAccount{Tenant: "cgrates.org",
		Account: "TestApierV2itSetActionWithCategory"}, &acnt); err != nil {
		t.Error(err)
	} else if len(acnt.BalanceMap) != 1 || acnt.BalanceMap[utils.MONETARY][0].Value != 5.0 {
		t.Errorf("Unexpected balance received: %+v", acnt.BalanceMap[utils.MONETARY][0])
	} else if len(acnt.BalanceMap[utils.MONETARY][0].Categories) != 1 &&
		acnt.BalanceMap[utils.MONETARY][0].Categories["test"] != true {
		t.Fatalf("Unexpected category received: %+v", utils.ToJSON(acnt))
	}
}

func TestApierV2itSetActionPlanWithWrongTiming(t *testing.T) {
	var reply string
	tNow := time.Now().Add(time.Duration(time.Minute)).String()
	argAP1 := &v1.AttrSetActionPlan{Id: "TestApierV2itSetAccountWithAPWithWrongTiming",
		ActionPlan: []*v1.AttrActionPlan{
			&v1.AttrActionPlan{
				ActionsId: "TestApierV2itSetAccountWithAP_ACT_1",
				Time:      tNow,
				Weight:    20.0,
			},
		},
	}

	if err := apierRPC.Call(utils.ApierV1SetActionPlan, argAP1, &reply); err == nil ||
		err.Error() != fmt.Sprintf("UNSUPPORTED_FORMAT:%s", tNow) {
		t.Error("Expecting error ", err)
	}
}

func TestApierV2itSetActionPlanWithWrongTiming2(t *testing.T) {
	var reply string
	argAP1 := &v1.AttrSetActionPlan{Id: "TestApierV2itSetAccountWithAPWithWrongTiming",
		ActionPlan: []*v1.AttrActionPlan{
			&v1.AttrActionPlan{
				ActionsId: "TestApierV2itSetAccountWithAP_ACT_1",
				Time:      "aa:bb:cc",
				Weight:    20.0,
			},
		},
	}

	if err := apierRPC.Call(utils.ApierV1SetActionPlan, argAP1, &reply); err == nil ||
		err.Error() != fmt.Sprintf("UNSUPPORTED_FORMAT:aa:bb:cc") {
		t.Error("Expecting error ", err)
	}
}

func TestApierV2itKillEngine(t *testing.T) {
	if err := engine.KillEngine(delay); err != nil {
		t.Error(err)
	}
}
