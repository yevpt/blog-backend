// Package moderation 持久化内容审核状态，并在单个 MySQL 事务中物化业务正文。
//
// 包内 subject adapter 是固定表映射，禁止由调用方传入表名或 GORM 回调。
// 幂等写使用 profile → moderation item → subject → revision 的固定加锁顺序；
// 首次创建没有可锁审核项，先校验父对象，再创建业务行和审核项。
package moderation
