// Copyright © 2024 Cisco Systems, Inc. and its affiliates.
// All rights reserved.
//
// Licensed under the Mozilla Public License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://mozilla.org/MPL/2.0/
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Regression test for https://github.com/CiscoDevNet/terraform-provider-meraki/issues/291:
// write-only attributes two levels deep in nested lists must be taken from the config.
func TestDeviceCellularSIMsWithWriteOnlyNestedLists(t *testing.T) {
	plan := DeviceCellularSIMs{Sims: []DeviceCellularSIMsSims{
		{Slot: types.StringValue("sim1"), Apns: []DeviceCellularSIMsSimsApns{
			{AuthenticationPasswordWo: types.StringNull()},
		}},
	}}
	config := DeviceCellularSIMs{Sims: []DeviceCellularSIMsSims{
		{Slot: types.StringValue("sim1"), Apns: []DeviceCellularSIMsSimsApns{
			{AuthenticationPasswordWo: types.StringValue("dummy-apn-password")},
		}},
	}}

	body := plan.withWriteOnly(config).toBody(context.Background(), DeviceCellularSIMs{})

	assertBodyValue(t, body, "sims.0.apns.0.authentication.password", "dummy-apn-password")
	if !plan.Sims[0].Apns[0].AuthenticationPasswordWo.IsNull() {
		t.Error("withWriteOnly modified the plan it was called on")
	}
}
