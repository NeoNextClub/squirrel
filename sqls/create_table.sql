create database if not exists mainnet character set UTF8mb4 collate utf8mb4_bin;
create database if not exists testnet character set UTF8mb4 collate utf8mb4_bin;


SET GLOBAL TX_ISOLATION = 'READ-COMMITTED';
SET GLOBAL BINLOG_FORMAT = 'ROW';


create table addr_asset
(
    id           int unsigned auto_increment primary key,
    address      varchar(128)             not null,
    asset_id     char(66)                 not null,
    balance      decimal(64, 22)          not null,
    transactions bigint unsigned          not null,
    last_transaction_time bigint unsigned not null
) engine = InnoDB default charset = 'utf8mb4';

create index addr_asset_asset_id_balance_index
    on addr_asset(asset_id, balance);

create unique index addr_asset_address_asset_id_uindex
    on addr_asset(address, asset_id);


create table addr_tx
(
    id         int unsigned auto_increment primary key,
    txid       char(66)        not null,
    address    varchar(128)        not null,
    block_time bigint unsigned not null,
    asset_type varchar(16)     not null
) engine = InnoDB default charset = 'utf8mb4';

create unique index addr_tx_address_asset_type_txid_uindex
    on addr_tx(address, asset_type, txid);

create index addr_tx_txid
    on addr_tx(txid);

create index addr_tx_address
    on addr_tx(address);


create table address
(
    id                    int unsigned auto_increment primary key,
    address               varchar(128)              not null,
    created_at            bigint unsigned           not null,
    last_transaction_time bigint unsigned           not null,
    trans_asset           bigint unsigned           not null,
    trans_nep5            bigint unsigned default 0 not null,
    trans_nft             bigint unsigned default 0 not null
) engine = InnoDB default charset = 'utf8mb4';

create unique index uk_address
    on address(address);


create table asset
(
    id           int unsigned auto_increment primary key,
    block_index  int unsigned     not null,
    block_time   bigint unsigned  not null,
    version      int unsigned     not null,
    asset_id     char(66)         not null,
    type         varchar(32)      not null,
    name         varchar(256)     not null,
    amount       decimal(35, 8)   not null,
    available    decimal(35, 8)   not null,
    `precision`  tinyint unsigned not null,
    owner        char(66)         not null,
    admin        char(34)         not null,
    issuer       char(66)         not null,
    expiration   bigint unsigned  not null,
    frozen       tinyint(1)       not null,
    addresses    bigint unsigned  not null,
    transactions bigint unsigned  not null,
    visible      tinyint(1) default 1 not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_asset_asset_id
    on asset(asset_id);

create index idx_asset_time
    on asset(block_time);


create table asset_tx
(
    id          int unsigned auto_increment primary key,
    address     char(34)        not null,
    asset_id    char(66)        not null,
    txid        char(66)        not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_asset_tx_address_asset_id
    on asset_tx(address, asset_id);

create unique index idx_asset_tx_address_asset_id_txid
    on asset_tx(address, asset_id, txid);


create table block
(
    id                  int unsigned auto_increment primary key,
    hash                char(66)        not null,
    size                int             not null,
    version             int unsigned    not null,
    previousblockhash   char(66)        not null,
    merkleroot          char(66)        not null,
    time                bigint unsigned not null,
    `index`             int unsigned    not null,
    nonce               char(16)        not null,
    nextconsensus       char(34)        not null,
    script_invocation   text            not null,
    script_verification text            not null,
    nextblockhash       char(66)        not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_block_hash
    on block(hash);

create unique index idx_block_index
    on block(`index`);

create index idx_block_time
    on block(time);


create table counter
(
    id                     int unsigned auto_increment primary key,
    last_block_index       int          not null,
    last_tx_pk             int unsigned not null,
    last_asset_tx_pk       int unsigned not null,
    last_tx_pk_for_nep5    int unsigned not null,
    app_log_idx            int          not null,
    last_tx_pk_for_nft     int unsigned not null,
    nft_app_log_idx        int          not null,
    last_tx_pk_for_sc      int unsigned not null,
    nep5_tx_pk_for_addr_tx int unsigned not null,
    nft_tx_pk_for_addr_tx  int unsigned not null,
    last_tx_pk_gas_balance int unsigned not null,
    cnt_addr               int unsigned not null,
    cnt_tx_reg             int unsigned not null,
    cnt_tx_miner           int unsigned not null,
    cnt_tx_issue           int unsigned not null,
    cnt_tx_invocation      int unsigned not null,
    cnt_tx_contract        int unsigned not null,
    cnt_tx_claim           int unsigned not null,
    cnt_tx_publish         int unsigned not null,
    cnt_tx_enrollment      int unsigned not null
) engine = InnoDB default charset = 'utf8mb4';


create table smartcontract_info
(
    id             int unsigned auto_increment primary key,
    txid           char(66)      not null,
    script_hash    char(40)      not null,
    name           varchar(255)  not null,
    version        varchar(255)  not null,
    author         varchar(255)  not null,
    email          varchar(255)  not null,
    description    varchar(2048) not null,
    need_storage   tinyint(1)    not null,
    parameter_list varchar(255)  not null,
    return_type    varchar(255)  not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_script_hash
    on smartcontract_info(script_hash);


create table nep5
(
    id                int unsigned auto_increment primary key,
    asset_id          char(40)             not null,
    admin_address     char(40)             not null,
    name              varchar(128)         not null,
    symbol            varchar(16)          not null,
    decimals          tinyint unsigned     not null,
    total_supply      decimal(64, 22)      not null,
    txid              char(66)             not null,
    block_index       int unsigned         not null,
    block_time        bigint unsigned      not null,
    addresses         bigint unsigned      not null,
    holding_addresses bigint unsigned      not null,
    transfers         bigint unsigned      not null,
    visible           tinyint(1) default 1 not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_nep5_txid
    on nep5(txid);


create table nep5_reg_info
(
    id             int unsigned auto_increment primary key,
    nep5_id        int unsigned  not null,
    name           varchar(255)  not null,
    version        varchar(255)  not null,
    author         varchar(255)  not null,
    email          varchar(255)  not null,
    description    varchar(1024) not null,
    need_storage   tinyint(1)    not null,
    parameter_list varchar(255)  not null,
    return_type    varchar(255)  not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_nep5_id
    on nep5_reg_info(nep5_id);


create table nep5_tx
(
    id          int unsigned auto_increment primary key,
    txid        char(66)        not null,
    asset_id    char(40)        not null,
    `from`      varchar(128)    not null,
    `to`        varchar(128)    not null,
    value       decimal(64, 22) not null,
    block_index int unsigned    not null,
    block_time  bigint unsigned not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_nep5_tx_asset_id
    on nep5_tx(asset_id);

create index idx_nep5_tx_from
    on nep5_tx(`from`);

create index idx_nep5_tx_to
    on nep5_tx(`to`);

create index idx_nep5_tx_txid
    on nep5_tx(txid);


create table nep5_migrate
(
    id           int unsigned auto_increment primary key,
    old_asset_id char(40) not null,
    new_asset_id char(40) not null,
    migrate_txid char(66) not null
) engine = InnoDB default charset = 'utf8mb4';


create table nft
(
    id                int unsigned auto_increment primary key,
    asset_id          char(40)             not null,
    admin_address     char(40)             not null,
    name              varchar(128)         not null,
    symbol            varchar(16)          not null,
    decimals          tinyint unsigned     not null,
    total_supply      decimal(35, 8)       not null,
    txid              char(66)             not null,
    block_index       int unsigned         not null,
    block_time        bigint unsigned      not null,
    addresses         bigint unsigned      not null,
    holding_addresses bigint unsigned      not null,
    transfers         bigint unsigned      not null,
    visible           tinyint(1) default 1 not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_nft_txid
    on nft(txid);


create table nft_reg_info
(
    id             int unsigned auto_increment primary key,
    nft_id         int unsigned not null,
    name           varchar(255) not null,
    version        varchar(255) not null,
    author         varchar(255) not null,
    email          varchar(255) not null,
    description    varchar(255) not null,
    need_storage   tinyint(1)   not null,
    parameter_list varchar(255) not null,
    return_type    varchar(255) not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_nft_id
    on nft_reg_info(nft_id);


create table nft_tx
(
    id          int unsigned auto_increment primary key,
    txid        char(66)        not null,
    asset_id    char(40)        not null,
    `from`      varchar(128)    not null,
    `to`        varchar(128)    not null,
    token_id    varchar(64)     not null,
    value       double          not null,
    block_index int unsigned    not null,
    block_time  bigint unsigned not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_nft_tx_asset_id
    on nft_tx(asset_id);

create index idx_nft_tx_from
    on nft_tx(`from`);

create index idx_nft_tx_to
    on nft_tx(`to`);

create index idx_nft_tx_txid
    on nft_tx(txid);


create table addr_asset_nft
(
    id           int unsigned auto_increment primary key,
    address      varchar(128)             not null,
    asset_id     char(66)                 not null,
    token_id     char(66)                 not null,
    balance      decimal(35, 8)           not null
) engine = InnoDB default charset = 'utf8mb4';


create table nft_token
(
    id           int unsigned auto_increment primary key,
    asset_id     char(66)                 not null,
    token_id     char(66)                 not null,
    info         varchar(4096)            not null
) engine = InnoDB default charset = 'utf8mb4';

create table tx
(
    id          int unsigned auto_increment primary key,
    block_index int unsigned    not null,
    block_time  bigint unsigned not null,
    txid        char(66)        not null,
    size        int unsigned    not null,
    type        varchar(32)     not null,
    version     int unsigned    not null,
    sys_fee     decimal(27, 8)  not null,
    net_fee     decimal(27, 8)  not null,
    nonce       bigint          not null,
    script      mediumtext      not null,
    gas         decimal(27, 8)  not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_tx_block_index
    on tx(block_index);

create index idx_tx_txid
    on tx(txid);

create index idx_tx_type
    on tx(type);


create table tx_attr
(
    id      int unsigned auto_increment primary key,
    txid    char(66)    not null,
    `usage` varchar(32) not null,
    data    mediumtext  not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_tx_attr_txid
    on tx_attr(txid);

create index idx_tx_attr_usage
    on tx_attr(`usage`);


create table tx_claims
(
    id   int unsigned auto_increment primary key,
    txid char(66)     not null,
    vout int unsigned not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_tx_claims_txid
    on tx_claims(txid);


create table tx_scripts
(
    id           int unsigned auto_increment primary key,
    txid         char(66) not null,
    invocation   text     not null,
    verification text     not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_tx_scripts_txid
    on tx_scripts(txid);


create table tx_vin
(
    id     int unsigned auto_increment primary key,
    `from` char(66)     not null,
    txid   char(66)     not null,
    vout   int unsigned not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_tx_vin_from
    on tx_vin(`from`);

create index idx_tx_vin_txid
    on tx_vin(txid);


create table tx_vout
(
    id       int unsigned auto_increment primary key,
    txid     char(66)       not null,
    n        int unsigned   not null,
    asset_id char(66)       not null,
    value    decimal(35, 8) not null,
    address  char(34)       not null
) engine = InnoDB default charset = 'utf8mb4';

create index idx_tx_vout_address
    on tx_vout(address);

create index idx_tx_vout_asset_id
    on tx_vout(asset_id);

create index idx_tx_vout_txid
    on tx_vout(txid);


create table utxo
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    txid       char(66)       not null,
    n          int unsigned   not null,
    asset_id   char(66)       not null,
    value      decimal(35, 8) not null,
    used_in_tx char(66)
) engine = InnoDB default charset = 'utf8mb4';

create index idx_utxo_address
    on utxo(address);

create index idx_utxo_asset_id
    on utxo(asset_id);

create index idx_utxo_txid
    on utxo(txid);

create index idx_utxo_used_in_tx
    on utxo(used_in_tx);

create table addr_gas_balance_a
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_a`(`address`, `date`);

create table addr_gas_balance_b
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_b`(`address`, `date`);

create table addr_gas_balance_c
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_c`(`address`, `date`);

create table addr_gas_balance_d
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_d`(`address`, `date`);

create table addr_gas_balance_e
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_e`(`address`, `date`);

create table addr_gas_balance_f
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_f`(`address`, `date`);

create table addr_gas_balance_g
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_g`(`address`, `date`);

create table addr_gas_balance_h
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_h`(`address`, `date`);

create table addr_gas_balance_i
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_i`(`address`, `date`);

create table addr_gas_balance_j
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_j`(`address`, `date`);

create table addr_gas_balance_k
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_k`(`address`, `date`);

create table addr_gas_balance_l
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_l`(`address`, `date`);

create table addr_gas_balance_m
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_m`(`address`, `date`);

create table addr_gas_balance_n
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_n`(`address`, `date`);

create table addr_gas_balance_o
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_o`(`address`, `date`);

create table addr_gas_balance_p
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_p`(`address`, `date`);

create table addr_gas_balance_q
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_q`(`address`, `date`);

create table addr_gas_balance_r
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_r`(`address`, `date`);

create table addr_gas_balance_s
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_s`(`address`, `date`);

create table addr_gas_balance_t
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_t`(`address`, `date`);

create table addr_gas_balance_u
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_u`(`address`, `date`);

create table addr_gas_balance_v
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_v`(`address`, `date`);

create table addr_gas_balance_w
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_w`(`address`, `date`);

create table addr_gas_balance_x
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_x`(`address`, `date`);

create table addr_gas_balance_y
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_y`(`address`, `date`);

create table addr_gas_balance_z
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_z`(`address`, `date`);

create table addr_gas_balance_0
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_0`(`address`, `date`);

create table addr_gas_balance_1
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_1`(`address`, `date`);

create table addr_gas_balance_2
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_2`(`address`, `date`);

create table addr_gas_balance_3
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_3`(`address`, `date`);

create table addr_gas_balance_4
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_4`(`address`, `date`);

create table addr_gas_balance_5
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_5`(`address`, `date`);

create table addr_gas_balance_6
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_6`(`address`, `date`);

create table addr_gas_balance_7
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_7`(`address`, `date`);

create table addr_gas_balance_8
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_8`(`address`, `date`);

create table addr_gas_balance_9
(
    id         int unsigned auto_increment primary key,
    address    char(34)       not null,
    date       date           not null,
    balance    decimal(35, 8) not null
) engine = InnoDB default charset = 'utf8mb4';

create index `idx_address_date`
    on `addr_gas_balance_9`(`address`, `date`);
