/*
 * Copyright (C) 2020 The poly network Authors
 * This file is part of The poly network library.
 *
 * The  poly network  is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * The  poly network  is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Lesser General Public License for more details.
 * You should have received a copy of the GNU Lesser General Public License
 * along with The poly network .  If not, see <http://www.gnu.org/licenses/>.
 */

package context

//import (
//	"encoding/json"
//	"io/ioutil"
//)

// actual json from api
//{
//	"prev_update_time":1617869441,
//	"details":{
//		"createWallet":{
//			"fee":"1714285714"
//		},
//		"deposit":{
//			"fee":"571428571"
//		},
//		"withdrawal":{
//			"fee":"1714285714"
//		}
//	}
//}
//
//type WithdrawalDetails struct {
//	Fee string `json:"fee"`
//}
//
//type FeeConfig struct {
//	Details WithdrawalDetails `json:"withdrawal"`
//}
//
//type Fees []map[string]string
//
//func NewConf(file string) (*Conf, error) {
//	conf := &FeeConfig{}
//	raw, err := ioutil.ReadFile(file)
//	if err != nil {
//		return nil, err
//	}
//	if err = json.Unmarshal(raw, conf); err != nil {
//		return nil, err
//	}
//	return conf, nil
//}
