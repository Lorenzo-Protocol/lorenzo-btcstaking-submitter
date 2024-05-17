CREATE TABLE `btc_deposit_tx` (
  `id` int NOT NULL AUTO_INCREMENT,
  `txid` varchar(256) NOT NULL,
  `receiver` varchar(256),
  `receiver_address` varchar(256),
  `amount` bigint,
  `status` tinyint NOT NULL,
  `height` bigint,
  `block_hash` varchar(256),
   `timestamp` datetime NOT NULL,
  `updated_time` datetime,
  `created_time` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY (`txid`),
  KEY (`timestamp`),
  KEY (`status`)
);

CREATE TABLE `config` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(256) NOT NULL,
  `value` varchar(256) NOT NULL,
  `created_time` datetime,
  `updated_time` datetime,
  PRIMARY KEY (`id`),
  UNIQUE KEY (`name`)
);