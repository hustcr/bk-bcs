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

package options

import (
	"bk-bcs/bcs-common/common/conf"
)

//ServerOption is option in flags
type ServerOption struct {
	conf.FileConfig
	conf.ServiceConfig
	conf.MetricConfig
	conf.ZkConfig
	conf.CertConfig
	conf.LicenseServerConfig
	conf.LogConfig
	conf.ProcessConfig
	conf.LocalConfig
	conf.CustomCertConfig

	VerifyClientTLS bool `json:"verify_client_tls" value:"false" usage:"verify client when brings up a tls server" mapstructure:"verify_client_tls"`

	BKIamAuth AuthOption `json:"bkiam_auth"`

	BKE BKEOptions `json:"bke"`

	Edition string `json:"edition" value:"ieod" usage:"api edition"`

	MesosWebconsoleProxyPort uint `json:"mesos_webconsole_proxy_port" value:"8083" usage:"Port to connect to mesos webconsole proxy"`
}

type BKEOptions struct {
	DSN                        string                     `json:"mysql_dsn" value:"" usage:"dsn for connect to mysql"`
	BootStrapUsers             []BootStrapUser            `json:"bootstrap_users"`
	ClusterCredentialsFixtures CredentialsFixturesOptions `json:"cluster_credentials_fixtures"`

	TurnOnRBAC bool `json:"turn_on_rbac" value:"false" usage:"turn on the rbac"`
	TurnOnAuth bool `json:"turn_on_auth" value:"false" usage:"turn on the auth"`
	TurnOnConf bool `json:"turn_on_conf" value:"false" usage:"turn on the conf"`

	RbacDatas []RbacData `json:"rbac_data"`
}

type RbacData struct {
	Username  string   `json:"user_name"`
	ClusterId string   `json:"cluster_id"`
	Roles     []string `json:"roles"`
}

type CredentialsFixturesOptions struct {
	Enabled     bool         `json:"is_enabled_fixtures_credentials"`
	Credentials []Credential `json:"credentials"`
}

type Credential struct {
	ClusterID string `json:"cluster_id"`
	Type      string `json:"type"`
	Server    string `json:"server"`
	CaCert    string `json:"ca_cert"`
	Token     string `json:"token"`
}

type BootStrapUser struct {
	Name        string   `json:"name"`
	IsSuperUser bool     `json:"is_super_user"`
	Tokens      []string `json:"tokens"`
}

type AuthOption struct {
	Auth bool `json:"auth" value:"false" usage:"use auth mode or not" mapstructure:"auth"`

	ApiGwRsaFile string `json:"apigw_rsa_file" value:"" usage:"apigw rsa public key file" mapstructure:"apigw_rsa_file"`

	AuthTokenSyncTime int `json:"auth_token_sync_time" value:"10" usage:"time ticker for syncing token in cache, seconds" mapstructure:"auth_token_sync_time"`

	BKIamAuthHost       string   `json:"bkiam_auth_host" value:"" usage:"bkiam auth server host" mapstructure:"bkiam_auth_host"`
	BKIamAuthAppCode    string   `json:"bkiam_auth_app_code" value:"" usage:"app code for communicating with auth" mapstructure:"bkiam_auth_app_code"`
	BKIamAuthAppSecret  string   `json:"bkiam_auth_app_secret" value:"" usage:"app secret for communicating with auth" mapstructure:"bkiam_auth_app_secret"`
	BKIamAuthSystemID   string   `json:"bkiam_auth_system_id" value:"" usage:"system id in auth service" mapstructure:"bkiam_auth_system_id"`
	BKIamAuthScopeID    string   `json:"bkiam_auth_scope_id" value:"" usage:"scope id in auth service" mapstructure:"bkiam_auth_scope_id"`
	BKIamZookeeper      string   `json:"bkiam_auth_zookeeper" value:"" usage:"zookeeper for auth token storage" mapstructure:"bkiam_auth_zookeeper"`
	BKIamTokenWhiteList []string `json:"bkiam_auth_token_whitelist" value:"" usage:"token whitelist for bkiam"`
	BKIamAuthSubServer  string   `json:"bkiam_auth_sub_server" value:"" usage:"bkiam auth subserver" mapstructure:"bkiam_auth_sub_server"`
}

//NewServerOption create a ServerOption object
func NewServerOption() *ServerOption {
	s := ServerOption{}
	return &s
}

func Parse(ops *ServerOption) error {
	conf.Parse(ops)
	return nil
}
