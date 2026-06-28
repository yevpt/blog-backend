// Package moderation 持久化内容审核状态，并在单个 MySQL 事务中物化业务正文。
//
// 包内 subject adapter 是固定表映射，禁止由调用方传入表名或 GORM 回调。
package moderation
