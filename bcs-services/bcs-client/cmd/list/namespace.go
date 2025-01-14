/*
 * Tencent is pleased to support the open source community by making Blueking Container Service available.
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package list

import (
	"bk-bcs/bcs-services/bcs-client/cmd/utils"
	"bk-bcs/bcs-services/bcs-client/pkg/storage/v1"
	"fmt"
)

func listNamespace(c *utils.ClientContext) error {
	if err := c.MustSpecified(utils.OptionClusterID); err != nil {
		return err
	}

	storage := v1.NewBcsStorage(utils.GetClientOption())
	list, err := storage.ListNamespace(c.ClusterID(), nil)
	if err != nil {
		return fmt.Errorf("failed to list namespace: %v", err)
	}

	return printListNamespace(list)
}

func printListNamespace(list []string) error {
	if len(list) == 0 {
		fmt.Printf("Found no namespace\n")
		return nil
	}

	fmt.Printf("%-5s %-20s\n", "INDEX", "NAMESPACE")
	for i, ns := range list {
		fmt.Printf("%-5d %-20s\n", i, ns)
	}
	return nil
}
