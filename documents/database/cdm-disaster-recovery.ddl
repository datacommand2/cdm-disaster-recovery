use cdm;

-- table
DROP TABLE IF EXISTS cdm_disaster_recovery_plan_instance_dependency;
DROP TABLE IF EXISTS cdm_disaster_recovery_plan_instance_floating_ip;
DROP TABLE IF EXISTS cdm_disaster_recovery_plan_volume;
DROP TABLE IF EXISTS cdm_disaster_recovery_plan_instance;
DROP TABLE IF EXISTS cdm_disaster_recovery_plan_tenant;
DROP TABLE IF EXISTS cdm_disaster_recovery_plan_availability_zone;
DROP TABLE IF EXISTS cdm_disaster_recovery_plan_external_network;
DROP TABLE IF EXISTS cdm_disaster_recovery_plan_external_routing_interface;
DROP TABLE IF EXISTS cdm_disaster_recovery_plan_router;
DROP TABLE IF EXISTS cdm_disaster_recovery_plan_storage;
DROP TABLE IF EXISTS cdm_disaster_recovery_job;
DROP TABLE IF EXISTS cdm_disaster_recovery_plan;
DROP TABLE IF EXISTS cdm_disaster_recovery_consistency_group_snapshot;
DROP TABLE IF EXISTS cdm_disaster_recovery_consistency_group;
DROP TABLE IF EXISTS cdm_disaster_recovery_protection_group_snapshot;
DROP TABLE IF EXISTS cdm_disaster_recovery_protection_instance;
DROP TABLE IF EXISTS cdm_disaster_recovery_protection_group;
DROP TABLE IF EXISTS cdm_disaster_recovery_instance_security_group;
DROP TABLE IF EXISTS cdm_disaster_recovery_subnet_dhcp_pool_result;
DROP TABLE IF EXISTS cdm_disaster_recovery_subnet_nameserver_result;
DROP TABLE IF EXISTS cdm_disaster_recovery_instance_network_result;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_instance_volume;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_instance_security_group;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_instance_network;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_instance_dependency;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_instance;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_volume;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_security_group_rule;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_security_group;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_extra_route;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_internal_routing_interface;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_external_routing_interface;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_router;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_floating_ip;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_subnet_nameserver;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_subnet_dhcp_pool;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_subnet;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_network;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_instance_extra_spec;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_instance_spec;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_keypair;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_tenant;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_task_log;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_failed_reason;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_warning_reason;
DROP TABLE IF EXISTS cdm_disaster_recovery_result_raw;
DROP TABLE IF EXISTS cdm_disaster_recovery_result;
DROP TABLE IF EXISTS cdm_disaster_recovery_instance_template_instance_dependency;
DROP TABLE IF EXISTS cdm_disaster_recovery_instance_template_instance;
DROP TABLE IF EXISTS cdm_disaster_recovery_instance_template;
-- sequence
DROP SEQUENCE IF EXISTS cdm_disaster_recovery_result_seq;
DROP SEQUENCE IF EXISTS cdm_disaster_recovery_job_seq;
DROP SEQUENCE IF EXISTS cdm_disaster_recovery_plan_seq;
DROP SEQUENCE IF EXISTS cdm_disaster_recovery_protection_group_snapshot_seq;
DROP SEQUENCE IF EXISTS cdm_disaster_recovery_protection_group_seq;
DROP SEQUENCE IF EXISTS cdm_disaster_recovery_instance_template_seq;

-- cdm_disaster_recovery_protection_group
CREATE SEQUENCE IF NOT EXISTS cdm_disaster_recovery_protection_group_seq;
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_protection_group (
    id INTEGER NOT NULL DEFAULT NEXTVAL('cdm_disaster_recovery_protection_group_seq'),
    owner_group_id INTEGER NOT NULL, -- cdm_group.id
    tenant_id INTEGER NOT NULL,
    protection_cluster_id INTEGER NOT NULL, -- cdm_cluster.id
    name VARCHAR(255) NOT NULL,
    remarks VARCHAR(300),
    recovery_point_objective_type VARCHAR(100) NOT NULL,
    recovery_point_objective INTEGER NOT NULL,
    recovery_time_objective INTEGER NOT NULL,
    snapshot_interval_type VARCHAR(100) NOT NULL,
    snapshot_interval INTEGER NOT NULL,
    snapshot_schedule_id INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (id)
);

-- cdm_disaster_recovery_protection_instance
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_protection_instance (
    protection_group_id INTEGER NOT NULL,
    protection_cluster_instance_id INTEGER NOT NULL, -- cdm_cluster_instance.id
    PRIMARY KEY (protection_group_id,protection_cluster_instance_id),
    CONSTRAINT cdm_disaster_recovery_protection_instance_protection_cluster_instance_id_un UNIQUE (protection_cluster_instance_id),
    CONSTRAINT cdm_disaster_recovery_protection_instance_protection_group_id_fk FOREIGN KEY (protection_group_id) REFERENCES cdm_disaster_recovery_protection_group (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_protection_group_snapshot
CREATE SEQUENCE IF NOT EXISTS cdm_disaster_recovery_protection_group_snapshot_seq;
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_protection_group_snapshot (
    id INTEGER NOT NULL DEFAULT NEXTVAL('cdm_disaster_recovery_protection_group_snapshot_seq'),
    protection_group_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT cdm_disaster_recovery_protection_group_snapshot_protection_group_id_fk FOREIGN KEY (protection_group_id) REFERENCES cdm_disaster_recovery_protection_group (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_consistency_group
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_consistency_group (
    protection_group_id INTEGER NOT NULL,
    protection_cluster_storage_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL,
    protection_cluster_volume_group_uuid VARCHAR(255) NOT NULL,
    PRIMARY KEY (protection_group_id, protection_cluster_storage_id, protection_cluster_tenant_id),
    CONSTRAINT cdm_disaster_recovery_consistency_group_protection_group_id_fk FOREIGN KEY (protection_group_id) REFERENCES cdm_disaster_recovery_protection_group (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_consistency_group_snapshot
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_consistency_group_snapshot (
    protection_group_snapshot_id INTEGER NOT NULL,
    protection_group_id INTEGER NOT NULL,
    protection_cluster_storage_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL,
    protection_cluster_volume_group_snapshot_uuid  VARCHAR(255) NOT NULL,
    PRIMARY KEY (protection_group_snapshot_id, protection_group_id, protection_cluster_storage_id, protection_cluster_tenant_id),
    CONSTRAINT cdm_disaster_recovery_consistency_group_snapshot_protection_group_snapshot_id_fk FOREIGN KEY (protection_group_snapshot_id) REFERENCES cdm_disaster_recovery_protection_group_snapshot (id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT cdm_disaster_recovery_consistency_group_snapshot_consistency_group_fk FOREIGN KEY (protection_group_id, protection_cluster_storage_id, protection_cluster_tenant_id) REFERENCES cdm_disaster_recovery_consistency_group (protection_group_id, protection_cluster_storage_id,  protection_cluster_tenant_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_plan
CREATE SEQUENCE IF NOT EXISTS cdm_disaster_recovery_plan_seq;
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_plan (
    id INTEGER NOT NULL DEFAULT NEXTVAL('cdm_disaster_recovery_plan_seq'),
    protection_group_id INTEGER NOT NULL,
    recovery_cluster_id INTEGER NOT NULL, -- cdm_cluster.id
    activation_flag BOOLEAN NOT NULL,
    name VARCHAR(255) NOT NULL,
    snapshot_retention_count INTEGER NOT NULL,
    remarks VARCHAR(300),
    direction_code VARCHAR(100) NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY	(id),
    CONSTRAINT cdm_disaster_recovery_plan_protection_group_id_fk FOREIGN KEY (protection_group_id) REFERENCES cdm_disaster_recovery_protection_group (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_plan_tenant
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_plan_tenant (
    recovery_plan_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    recovery_type_code VARCHAR(100) NOT NULL,
    recovery_cluster_tenant_id INTEGER, -- cdm_cluster_tenant.id
    recovery_cluster_tenant_mirror_name VARCHAR(255),
    recovery_cluster_tenant_mirror_name_update_flag BOOLEAN DEFAULT false,
    recovery_cluster_tenant_mirror_name_update_reason_code VARCHAR(100),
    recovery_cluster_tenant_mirror_name_update_reason_contents TEXT,
    PRIMARY KEY(recovery_plan_id, protection_cluster_tenant_id),
    CONSTRAINT cdm_disaster_recovery_plan_tenant_recovery_plan_id_fk FOREIGN KEY (recovery_plan_id) REFERENCES cdm_disaster_recovery_plan (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_plan_availability_zone
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_plan_availability_zone (
    recovery_plan_id INTEGER NOT NULL,
    protection_cluster_availability_zone_id INTEGER NOT NULL, -- cdm_cluster_availability_zone.id
    recovery_type_code VARCHAR(100) NOT NULL,
    recovery_cluster_availability_zone_id INTEGER,
    recovery_cluster_availability_zone_update_flag BOOLEAN DEFAULT false,
    recovery_cluster_availability_zone_update_reason_code VARCHAR(100),
    recovery_cluster_availability_zone_update_reason_contents TEXT,
    PRIMARY KEY(recovery_plan_id, protection_cluster_availability_zone_id),
    CONSTRAINT cdm_disaster_recovery_plan_availability_zone_recovery_plan_id_fk FOREIGN KEY (recovery_plan_id) REFERENCES cdm_disaster_recovery_plan (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_plan_external_network
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_plan_external_network (
    recovery_plan_id INTEGER NOT NULL,
    protection_cluster_external_network_id INTEGER NOT NULL, -- cdm_cluster_network.id
    recovery_type_code VARCHAR(100) NOT NULL,
    recovery_cluster_external_network_id INTEGER,
    recovery_cluster_external_network_update_flag BOOLEAN DEFAULT false,
    recovery_cluster_external_network_update_reason_code VARCHAR(100),
    recovery_cluster_external_network_update_reason_contents TEXT,
    PRIMARY KEY(recovery_plan_id, protection_cluster_external_network_id),
    CONSTRAINT cdm_disaster_recovery_plan_external_network_recovery_plan_id_fk FOREIGN KEY (recovery_plan_id) REFERENCES cdm_disaster_recovery_plan (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_plan_router
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_plan_router (
    recovery_plan_id INTEGER NOT NULL,
    protection_cluster_router_id INTEGER NOT NULL, -- cdm_cluster_router.id
    recovery_type_code VARCHAR(100) NOT NULL,
    recovery_cluster_external_network_id INTEGER,
    recovery_cluster_external_network_update_flag BOOLEAN DEFAULT false,
    recovery_cluster_external_network_update_reason_code VARCHAR(100),
    recovery_cluster_external_network_update_reason_contents TEXT,
    PRIMARY KEY(recovery_plan_id, protection_cluster_router_id),
    CONSTRAINT cdm_disaster_recovery_plan_router_recovery_plan_id_fk FOREIGN KEY (recovery_plan_id) REFERENCES cdm_disaster_recovery_plan (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_plan_external_routing_interface
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_plan_external_routing_interface (
    recovery_plan_id INTEGER NOT NULL,
    protection_cluster_router_id INTEGER NOT NULL,
    recovery_cluster_external_subnet_id INTEGER NOT NULL, -- cdm_cluster_subnet.id
    ip_address VARCHAR(40),
    PRIMARY KEY(recovery_plan_id, protection_cluster_router_id, recovery_cluster_external_subnet_id),
    CONSTRAINT cdm_disaster_recovery_plan_external_routing_interface_recovery_plan_router_fk FOREIGN KEY (recovery_plan_id, protection_cluster_router_id) REFERENCES cdm_disaster_recovery_plan_router (recovery_plan_id, protection_cluster_router_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_plan_storage
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_plan_storage (
    recovery_plan_id INTEGER NOT NULL,
    protection_cluster_storage_id INTEGER NOT NULL, -- cdm_cluster_storage.id
    recovery_type_code VARCHAR(100) NOT NULL,
    recovery_cluster_storage_id INTEGER,
    recovery_cluster_storage_update_flag BOOLEAN DEFAULT false,
    recovery_cluster_storage_update_reason_code VARCHAR(100),
    recovery_cluster_storage_update_reason_contents TEXT,
    unavailable_flag BOOLEAN DEFAULT false,
    unavailable_reason_code VARCHAR(100),
    unavailable_reason_contents TEXT,
    PRIMARY KEY(recovery_plan_id, protection_cluster_storage_id),
    CONSTRAINT cdm_disaster_recovery_plan_storage_recovery_plan_id_fk FOREIGN KEY (recovery_plan_id) REFERENCES cdm_disaster_recovery_plan (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_plan_instance
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_plan_instance (
    recovery_plan_id INTEGER NOT NULL,
    protection_cluster_instance_id INTEGER NOT NULL, -- cdm_cluster_instance.id
    recovery_type_code VARCHAR(100) NOT NULL,
    recovery_cluster_availability_zone_id INTEGER,
    recovery_cluster_availability_zone_update_flag BOOLEAN DEFAULT false,
    recovery_cluster_availability_zone_update_reason_code VARCHAR(100),
    recovery_cluster_availability_zone_update_reason_contents TEXT,
    recovery_cluster_hypervisor_id INTEGER,
    auto_start_flag BOOLEAN DEFAULT false,
    diagnosis_flag BOOLEAN DEFAULT false,
    diagnosis_method_code VARCHAR(100),
    diagnosis_method_data VARCHAR(300),
    diagnosis_timeout INTEGER ,
    PRIMARY KEY (recovery_plan_id, protection_cluster_instance_id),
    CONSTRAINT cdm_disaster_recovery_plan_instance_recovery_plan_id_fk FOREIGN KEY (recovery_plan_id) REFERENCES cdm_disaster_recovery_plan (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_plan_instance_dependency
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_plan_instance_dependency (
    recovery_plan_id INTEGER NOT NULL,
    protection_cluster_instance_id INTEGER NOT NULL, -- cdm_cluster_instance.id
    depend_protection_cluster_instance_id INTEGER NOT NULL, -- cdm_cluster_instance.id
    PRIMARY KEY (recovery_plan_id, protection_cluster_instance_id, depend_protection_cluster_instance_id),
    CONSTRAINT cdm_disaster_recovery_plan_instance_dependency_recovery_plan_id_fk FOREIGN KEY (recovery_plan_id) REFERENCES cdm_disaster_recovery_plan (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_plan_instance_floating_ip
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_plan_instance_floating_ip (
    recovery_plan_id INTEGER NOT NULL,
    protection_cluster_instance_id INTEGER NOT NULL, -- cdm_cluster_instance.id
    protection_cluster_instance_floating_ip_id INTEGER NOT NULL, -- cdm_cluster_instance_floating_ip.id
    unavailable_flag BOOLEAN,
    unavailable_reason_code VARCHAR(100),
    unavailable_reason_contents TEXT,
    PRIMARY KEY (recovery_plan_id, protection_cluster_instance_id, protection_cluster_instance_floating_ip_id),
    CONSTRAINT cdm_disaster_recovery_plan_instance_floating_ip_recovery_plan_id_fk FOREIGN KEY (recovery_plan_id) REFERENCES cdm_disaster_recovery_plan (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_plan_volume
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_plan_volume (
    recovery_plan_id INTEGER NOT NULL,
    protection_cluster_volume_id INTEGER NOT NULL, -- cdm_cluster_volume.id
    recovery_type_code VARCHAR(100) NOT NULL,
    recovery_cluster_storage_id INTEGER,
    recovery_cluster_storage_update_flag BOOLEAN DEFAULT false,
    recovery_cluster_storage_update_reason_code VARCHAR(100),
    recovery_cluster_storage_update_reason_contents TEXT,
    unavailable_flag BOOLEAN DEFAULT false,
    unavailable_reason_code VARCHAR(100),
    unavailable_reason_contents TEXT,
    PRIMARY KEY (recovery_plan_id, protection_cluster_volume_id),
    CONSTRAINT cdm_disaster_recovery_plan_volume_recovery_plan_id_fk FOREIGN KEY (recovery_plan_id) REFERENCES cdm_disaster_recovery_plan (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_job
CREATE SEQUENCE IF NOT EXISTS cdm_disaster_recovery_job_seq;
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_job (
    id INTEGER NOT NULL DEFAULT NEXTVAL('cdm_disaster_recovery_job_seq'),
    recovery_plan_id INTEGER NOT NULL,
    type_code VARCHAR(100) NOT NULL,
    recovery_point_type_code VARCHAR(100) NOT NULL,
    schedule_id INTEGER, -- cdm_schedule.id
    next_runtime INTEGER NOT NULL,
    recovery_point_snapshot_id INTEGER,
    operator_id INTEGER NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT cdm_disaster_recovery_job_recovery_plan_id_fk FOREIGN KEY (recovery_plan_id) REFERENCES cdm_disaster_recovery_plan (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result
CREATE SEQUENCE IF NOT EXISTS cdm_disaster_recovery_result_seq;
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result (
    id INTEGER NOT NULL DEFAULT NEXTVAL('cdm_disaster_recovery_result_seq'),
    owner_group_id INTEGER NOT NULL, -- cdm_group.id
    operator_id INTEGER NOT NULL, -- cdm_user.id
    operator_account VARCHAR(30) NOT NULL, -- cdm_user.account
    operator_name VARCHAR(255) NOT NULL, -- cdm_user.name
    operator_department VARCHAR(255), -- cdm_user.department
    operator_position VARCHAR(255), -- cdm_user.position
    approver_id INTEGER, -- cdm_user.id
    approver_account VARCHAR(30), -- cdm_user.account
    approver_name VARCHAR(255), -- cdm_user.name
    approver_department VARCHAR(255), -- cdm_user.department
    approver_position VARCHAR(255), -- cdm_user.position
    protection_group_id INTEGER NOT NULL, -- cdm_disaster_recovery_protection_group.id
    protection_group_name VARCHAR(255) NOT NULL, -- cdm_disaster_recovery_protection_group.name
    protection_group_remarks VARCHAR(300), -- cdm_disaster_recovery_protection_group.remarks
    protection_cluster_id INTEGER NOT NULL, -- cdm_cluster.id
    protection_cluster_type_code VARCHAR(100) NOT NULL, -- cdm_cluster.type_code
    protection_cluster_name VARCHAR(255) NOT NULL, -- cdm_cluster.name
    protection_cluster_remarks VARCHAR(300), -- cdm_cluster.remarks
    recovery_plan_id INTEGER NOT NULL, -- cdm_disaster_recovery_plan.id
    recovery_plan_name VARCHAR(255) NOT NULL, -- cdm_disaster_recovery_plan.name
    recovery_plan_remarks VARCHAR(300), -- cdm_disaster_recovery_plan.remarks
    recovery_cluster_id INTEGER NOT NULL, -- cdm_cluster.id
    recovery_cluster_type_code VARCHAR(100) NOT NULL, -- cdm_cluster.type_code
    recovery_cluster_name VARCHAR(255) NOT NULL, -- cdm_cluster.name
    recovery_cluster_remarks VARCHAR(300), -- cdm_cluster.remarks
    recovery_type_code VARCHAR(100) NOT NULL,
    recovery_direction_code VARCHAR(100) NOT NULL,
    recovery_point_objective_type VARCHAR(100) NOT NULL,
    recovery_point_objective INTEGER NOT NULL,
    recovery_time_objective INTEGER NOT NULL,
    recovery_point_type_code VARCHAR(100) NOT NULL,
    recovery_point INTEGER NOT NULL,
    schedule_type VARCHAR(100),
    started_at INTEGER NOT NULL,
    finished_at INTEGER NOT NULL,
    elapsed_time INTEGER NOT NULL,
    result_code VARCHAR(100) NOT NULL,
    warning_flag BOOLEAN NOT NULL,
    PRIMARY KEY(id)
);

-- cdm_disaster_recovery_result_raw
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_raw (
   recovery_result_id INTEGER NOT NULL,
   recovery_job_id INTEGER NOT NULL,
   contents TEXT,
   shared_task TEXT,
   PRIMARY KEY (recovery_result_id),
   CONSTRAINT cdm_disaster_recovery_result_task_log_recovery_result_id_fk FOREIGN KEY (recovery_result_id) REFERENCES cdm_disaster_recovery_result (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_warning_reason
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_warning_reason (
    recovery_result_id INTEGER NOT NULL,
    reason_seq INTEGER NOT NULL,
    code VARCHAR(100) NOT NULL,
    contents TEXT,
    PRIMARY KEY(recovery_result_id, reason_seq),
    CONSTRAINT cdm_disaster_recovery_result_warning_reason_recovery_result_id_fk FOREIGN KEY (recovery_result_id) REFERENCES cdm_disaster_recovery_result (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_failed_reason
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_failed_reason (
    recovery_result_id INTEGER NOT NULL,
    reason_seq INTEGER NOT NULL,
    code VARCHAR(100) NOT NULL,
    contents TEXT,
    PRIMARY KEY(recovery_result_id, reason_seq),
    CONSTRAINT cdm_disaster_recovery_result_failed_reason_recovery_result_id_fk FOREIGN KEY (recovery_result_id) REFERENCES cdm_disaster_recovery_result (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_task_log
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_task_log (
    recovery_result_id INTEGER NOT NULL,
    log_seq INTEGER NOT NULL,
    code VARCHAR(100) NOT NULL,
    contents TEXT,
    log_dt INTEGER NOT NULL,
    PRIMARY KEY (recovery_result_id, log_seq),
    CONSTRAINT cdm_disaster_recovery_result_task_log_recovery_result_id_fk FOREIGN KEY (recovery_result_id) REFERENCES cdm_disaster_recovery_result (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_tenant
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_tenant (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_tenant_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_tenant.uuid
    protection_cluster_tenant_name VARCHAR(255) NOT NULL, -- cdm_cluster_tenant.name
    protection_cluster_tenant_description TEXT, -- cdm_cluster_tenant.description
    protection_cluster_tenant_enabled BOOLEAN NOT NULL, -- cdm_cluster_tenant.enabled
    recovery_cluster_tenant_id INTEGER, -- cdm_cluster_tenant.id
    recovery_cluster_tenant_uuid VARCHAR(36), -- cdm_cluster_tenant.uuid
    recovery_cluster_tenant_name VARCHAR(255), -- cdm_cluster_tenant.name
    recovery_cluster_tenant_description TEXT, -- cdm_cluster_tenant.description
    recovery_cluster_tenant_enabled BOOLEAN, -- cdm_cluster_tenant.enabled
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id),
    CONSTRAINT cdm_disaster_recovery_result_tenant_recovery_result_id_fk FOREIGN KEY (recovery_result_id) REFERENCES cdm_disaster_recovery_result (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_instance_spec
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_instance_spec (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_instance_spec_id INTEGER NOT NULL, -- cdm_cluster_instance_spec.id
    protection_cluster_instance_spec_uuid VARCHAR(255) NOT NULL, -- cdm_cluster_instance_spec.uuid
    protection_cluster_instance_spec_name VARCHAR(255) NOT NULL, -- cdm_cluster_instance_spec.name
    protection_cluster_instance_spec_description VARCHAR(255), -- cdm_cluster_instance_spec.description
    protection_cluster_instance_spec_vcpu_total_cnt INTEGER NOT NULL, -- cdm_cluster_instance_spec.vcpu_total_cnt
    protection_cluster_instance_spec_mem_total_bytes INTEGER NOT NULL, -- cdm_cluster_instance_spec.mem_total_bytes
    protection_cluster_instance_spec_disk_total_bytes INTEGER NOT NULL, -- cdm_cluster_instance_spec.disk_total_bytes
    protection_cluster_instance_spec_swap_total_bytes INTEGER NOT NULL, -- cdm_cluster_instance_spec.swap_total_bytes
    protection_cluster_instance_spec_ephemeral_total_bytes INTEGER NOT NULL, -- cdm_cluster_instance_spec.ephemeral_total_bytes
    recovery_cluster_instance_spec_id INTEGER, -- cdm_cluster_instance_spec.id
    recovery_cluster_instance_spec_uuid VARCHAR(255), -- cdm_cluster_instance_spec.uuid
    recovery_cluster_instance_spec_name VARCHAR(255), -- cdm_cluster_instance_spec.name
    recovery_cluster_instance_spec_description VARCHAR(255), -- cdm_cluster_instance_spec.description
    recovery_cluster_instance_spec_vcpu_total_cnt INTEGER, -- cdm_cluster_instance_spec.vcpu_total_cnt
    recovery_cluster_instance_spec_mem_total_bytes INTEGER, -- cdm_cluster_instance_spec.mem_total_bytes
    recovery_cluster_instance_spec_disk_total_bytes INTEGER, -- cdm_cluster_instance_spec.disk_total_bytes
    recovery_cluster_instance_spec_swap_total_bytes INTEGER, -- cdm_cluster_instance_spec.swap_total_bytes
    recovery_cluster_instance_spec_ephemeral_total_bytes INTEGER, -- cdm_cluster_instance_spec.ephemeral_total_bytes
    PRIMARY KEY (recovery_result_id, protection_cluster_instance_spec_id),
    CONSTRAINT cdm_disaster_recovery_result_instance_spec_recovery_result_id_fk FOREIGN KEY (recovery_result_id) REFERENCES cdm_disaster_recovery_result (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_instance_extra_spec
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_instance_extra_spec (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_instance_spec_id INTEGER NOT NULL,
    protection_cluster_instance_extra_spec_id INTEGER NOT NULL, -- cdm_cluster_instance_extra_spec.id
    protection_cluster_instance_extra_spec_key VARCHAR(255) NOT NULL, -- cdm_cluster_instance_extra_spec.key
    protection_cluster_instance_extra_spec_value VARCHAR(255) NOT NULL, -- cdm_cluster_instance_extra_spec.value
    recovery_cluster_instance_extra_spec_id INTEGER, -- cdm_cluster_instance_extra_spec.id
    recovery_cluster_instance_extra_spec_key VARCHAR(255), -- cdm_cluster_instance_extra_spec.key
    recovery_cluster_instance_extra_spec_value VARCHAR(255), -- cdm_cluster_instance_extra_spec.value
    PRIMARY KEY (recovery_result_id, protection_cluster_instance_spec_id, protection_cluster_instance_extra_spec_id),
    CONSTRAINT cdm_disaster_recovery_result_instance_extra_spec_instance_spec_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_instance_spec_id) REFERENCES cdm_disaster_recovery_result_instance_spec (recovery_result_id, protection_cluster_instance_spec_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_keypair
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_keypair (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_keypair_id INTEGER NOT NULL, -- cdm_cluster_keypair.id
    protection_cluster_keypair_name VARCHAR(255) NOT NULL, -- cdm_cluster_keypair.name
    protection_cluster_keypair_fingerprint VARCHAR(100) NOT NULL, -- cdm_cluster_keypair.fingerprint
    protection_cluster_keypair_public_key VARCHAR(2048) NOT NULL, -- cdm_cluster_keypair.public_key
    protection_cluster_keypair_type_code VARCHAR(50) NOT NULL, -- cdm_cluster_keypair.type_code
    recovery_cluster_keypair_id INTEGER, -- cdm_cluster_keypair.id
    recovery_cluster_keypair_name VARCHAR(255), -- cdm_cluster_keypair.name
    recovery_cluster_keypair_fingerprint VARCHAR(100), -- cdm_cluster_keypair.fingerprint
    recovery_cluster_keypair_public_key VARCHAR(2048), -- cdm_cluster_keypair.public_key
    recovery_cluster_keypair_type_code VARCHAR(50), -- cdm_cluster_keypair.type_code
    PRIMARY KEY (recovery_result_id, protection_cluster_keypair_id),
    CONSTRAINT cdm_disaster_recovery_result_keypair_recovery_result_id_fk FOREIGN KEY (recovery_result_id) REFERENCES cdm_disaster_recovery_result (id) ON UPDATE RESTRICT ON DELETE RESTRICT
    );

-- cdm_disaster_recovery_result_network
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_network (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_network_id INTEGER NOT NULL, -- cdm_cluster_network.id
    protection_cluster_network_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_network.uuid
    protection_cluster_network_name VARCHAR(255) NOT NULL, -- cdm_cluster_network.name
    protection_cluster_network_description VARCHAR(255), -- cdm_cluster_network.description
    protection_cluster_network_type_code VARCHAR(100) NOT NULL, -- cdm_cluster_network.type_code
    protection_cluster_network_state VARCHAR(20) NOT NULL, -- cdm_cluster_network.state
    recovery_cluster_network_id INTEGER, -- cdm_cluster_network.id
    recovery_cluster_network_uuid VARCHAR(36), -- cdm_cluster_network.uuid
    recovery_cluster_network_name VARCHAR(255), -- cdm_cluster_network.name
    recovery_cluster_network_description VARCHAR(255), -- cdm_cluster_network.description
    recovery_cluster_network_type_code VARCHAR(100), -- cdm_cluster_network.type_code
    recovery_cluster_network_state VARCHAR(20), -- cdm_cluster_network.state
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_network_id),
    CONSTRAINT cdm_disaster_recovery_result_network_tenant_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id) REFERENCES cdm_disaster_recovery_result_tenant (recovery_result_id, protection_cluster_tenant_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_subnet
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_subnet (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_network_id INTEGER NOT NULL, -- cdm_cluster_network.id
    protection_cluster_subnet_id INTEGER NOT NULL, -- cdm_cluster_subnet.id
    protection_cluster_subnet_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_subnet.uuid
    protection_cluster_subnet_name VARCHAR(255) NOT NULL, -- cdm_cluster_subnet.name
    protection_cluster_subnet_description VARCHAR(255), -- cdm_cluster_subnet.description
    protection_cluster_subnet_network_cidr VARCHAR(64) NOT NULL, -- cdm_cluster_subnet.network_cidr
    protection_cluster_subnet_dhcp_enabled BOOLEAN NOT NULL, -- cdm_cluster_subnet.dhcp_enabled
    protection_cluster_subnet_gateway_enabled BOOLEAN NOT NULL, -- cdm_cluster_subnet.gateway_enabled
    protection_cluster_subnet_gateway_ip_address VARCHAR(40), -- cdm_cluster_subnet.gateway_ip_address
    protection_cluster_subnet_ipv6_address_mode_code VARCHAR(100), -- cdm_cluster_subnet.ipv6_address_mode_code
    protection_cluster_subnet_ipv6_ra_mode_code VARCHAR(100), -- cdm_cluster_subnet.ipv6_ra_mode_code
    recovery_cluster_subnet_id INTEGER, -- cdm_cluster_subnet.id
    recovery_cluster_subnet_uuid VARCHAR(36), -- cdm_cluster_subnet.uuid
    recovery_cluster_subnet_name VARCHAR(255), -- cdm_cluster_subnet.name
    recovery_cluster_subnet_description VARCHAR(255), -- cdm_cluster_subnet.description
    recovery_cluster_subnet_network_cidr VARCHAR(64), -- cdm_cluster_subnet.network_cidr
    recovery_cluster_subnet_dhcp_enabled BOOLEAN, -- cdm_cluster_subnet.dhcp_enabled
    recovery_cluster_subnet_gateway_enabled BOOLEAN, -- cdm_cluster_subnet.gateway_enabled
    recovery_cluster_subnet_gateway_ip_address VARCHAR(40), -- cdm_cluster_subnet.gateway_ip_address
    recovery_cluster_subnet_ipv6_address_mode_code VARCHAR(100), -- cdm_cluster_subnet.ipv6_address_mode_code
    recovery_cluster_subnet_ipv6_ra_mode_code VARCHAR(100), -- cdm_cluster_subnet.ipv6_ra_mode_code
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_network_id, protection_cluster_subnet_id),
    CONSTRAINT cdm_disaster_recovery_result_subnet_network_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_network_id) REFERENCES cdm_disaster_recovery_result_network (recovery_result_id, protection_cluster_tenant_id, protection_cluster_network_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_subnet_dhcp_pool
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_subnet_dhcp_pool (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_network_id INTEGER NOT NULL, -- cdm_cluster_network.id
    protection_cluster_subnet_id INTEGER NOT NULL, -- cdm_cluster_subnet.id
    dhcp_pool_seq INTEGER NOT NULL,
    protection_cluster_start_ip_address VARCHAR(40) NOT NULL, -- cdm_cluster_subnet_dhcp_pool.start_ip_address
    protection_cluster_end_ip_address VARCHAR(40) NOT NULL, -- cdm_cluster_subnet_dhcp_pool.end_ip_address
    recovery_cluster_start_ip_address VARCHAR(40), -- cdm_cluster_subnet_dhcp_pool.start_ip_address
    recovery_cluster_end_ip_address VARCHAR(40), -- cdm_cluster_subnet_dhcp_pool.end_ip_address
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_network_id, protection_cluster_subnet_id, dhcp_pool_seq),
    CONSTRAINT cdm_disaster_recovery_result_subnet_dhcp_pool_subnet_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_network_id, protection_cluster_subnet_id) REFERENCES cdm_disaster_recovery_result_subnet (recovery_result_id, protection_cluster_tenant_id, protection_cluster_network_id, protection_cluster_subnet_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_subnet_nameserver
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_subnet_nameserver (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_network_id INTEGER NOT NULL, -- cdm_cluster_network.id
    protection_cluster_subnet_id INTEGER NOT NULL, -- cdm_cluster_subnet.id
    nameserver_seq INTEGER NOT NULL,
    protection_cluster_nameserver VARCHAR(255) NOT NULL, -- cdm_cluster_subnet_nameserver.nameserver
    recovery_cluster_nameserver VARCHAR(255), -- cdm_cluster_subnet_nameserver.nameserver
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_network_id, protection_cluster_subnet_id, nameserver_seq),
    CONSTRAINT cdm_disaster_recovery_result_subnet_nameserver_subnet_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_network_id, protection_cluster_subnet_id) REFERENCES cdm_disaster_recovery_result_subnet (recovery_result_id, protection_cluster_tenant_id, protection_cluster_network_id, protection_cluster_subnet_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_floating_ip
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_floating_ip (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_floating_ip_id INTEGER NOT NULL, -- cdm_cluster_floating_ip.id
    protection_cluster_floating_ip_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_floating_ip.uuid
    protection_cluster_floating_ip_description VARCHAR(255), -- cdm_cluster_floating_ip.description
    protection_cluster_floating_ip_ip_address VARCHAR(40) NOT NULL, -- cdm_cluster_floating_ip.ip_address
    protection_cluster_network_id INTEGER NOT NULL, -- cdm_cluster_network.id
    protection_cluster_network_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_network.uuid
    protection_cluster_network_name VARCHAR(255) NOT NULL, -- cdm_cluster_network.name
    protection_cluster_network_description VARCHAR(255), -- cdm_cluster_network.description
    protection_cluster_network_type_code VARCHAR(100) NOT NULL, -- cdm_cluster_network.type_code
    protection_cluster_network_state VARCHAR(20) NOT NULL, -- cdm_cluster_network.state
    recovery_cluster_floating_ip_id INTEGER, -- cdm_cluster_floating_ip.id
    recovery_cluster_floating_ip_uuid VARCHAR(36), -- cdm_cluster_floating_ip.uuid
    recovery_cluster_floating_ip_description VARCHAR(255), -- cdm_cluster_floating_ip.description
    recovery_cluster_floating_ip_ip_address VARCHAR(40), -- cdm_cluster_floating_ip.ip_address
    recovery_cluster_network_id INTEGER, -- cdm_cluster_network.id
    recovery_cluster_network_uuid VARCHAR(36), -- cdm_cluster_network.uuid
    recovery_cluster_network_name VARCHAR(255), -- cdm_cluster_network.name
    recovery_cluster_network_description VARCHAR(255), -- cdm_cluster_network.description
    recovery_cluster_network_type_code VARCHAR(100), -- cdm_cluster_network.type_code
    recovery_cluster_network_state VARCHAR(20), -- cdm_cluster_network.state
    PRIMARY KEY (recovery_result_id, protection_cluster_floating_ip_id),
    CONSTRAINT cdm_disaster_recovery_result_tenant_recovery_result_id_fk FOREIGN KEY (recovery_result_id) REFERENCES cdm_disaster_recovery_result (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_router
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_router (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_router_id INTEGER NOT NULL, -- cdm_cluster_router.id
    protection_cluster_router_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_router.uuid
    protection_cluster_router_name VARCHAR(255) NOT NULL, -- cdm_cluster_router.name
    protection_cluster_router_description VARCHAR(255), -- cdm_cluster_router.description
    protection_cluster_router_state VARCHAR(20) NOT NULL, -- cdm_cluster_router.state
    recovery_cluster_router_id INTEGER, -- cdm_cluster_router.id
    recovery_cluster_router_uuid VARCHAR(36), -- cdm_cluster_router.uuid
    recovery_cluster_router_name VARCHAR(255), -- cdm_cluster_router.name
    recovery_cluster_router_description VARCHAR(255), -- cdm_cluster_router.description
    recovery_cluster_router_state VARCHAR(20), -- cdm_cluster_router.state
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_router_id),
    CONSTRAINT cdm_disaster_recovery_result_router_tenant_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id) REFERENCES cdm_disaster_recovery_result_tenant (recovery_result_id, protection_cluster_tenant_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_internal_routing_interface
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_internal_routing_interface (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_router_id INTEGER NOT NULL, -- cdm_cluster_router.id
    routing_interface_seq INTEGER NOT NULL,
    protection_cluster_network_id INTEGER NOT NULL, -- cdm_cluster_network.id
    protection_cluster_subnet_id INTEGER NOT NULL, -- cdm_cluster_subnet.id
    protection_cluster_ip_address VARCHAR(40) NOT NULL,
    recovery_cluster_network_id INTEGER, -- cdm_cluster_network.id
    recovery_cluster_subnet_id INTEGER, -- cdm_cluster_subnet.id
    recovery_cluster_ip_address VARCHAR(40),
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_router_id, routing_interface_seq),
    CONSTRAINT cdm_disaster_recovery_result_internal_routing_interface_router_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_router_id) REFERENCES cdm_disaster_recovery_result_router (recovery_result_id, protection_cluster_tenant_id, protection_cluster_router_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_external_routing_interface
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_external_routing_interface (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_router_id INTEGER NOT NULL, -- cdm_cluster_router.id
    routing_interface_seq INTEGER NOT NULL,
    cluster_network_id INTEGER NOT NULL, -- cdm_cluster_network.id
    cluster_network_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_network.uuid
    cluster_network_name VARCHAR(255) NOT NULL, -- cdm_cluster_network.name
    cluster_network_description VARCHAR(255), -- cdm_cluster_network.description
    cluster_network_type_code VARCHAR(100) NOT NULL, -- cdm_cluster_network.type_code
    cluster_network_state VARCHAR(20) NOT NULL, -- cdm_cluster_network.state
    cluster_subnet_id INTEGER NOT NULL, -- cdm_cluster_subnet.id
    cluster_subnet_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_subnet.uuid
    cluster_subnet_name VARCHAR(255) NOT NULL, -- cdm_cluster_subnet.name
    cluster_subnet_description VARCHAR(255), -- cdm_cluster_subnet.description
    cluster_subnet_network_cidr VARCHAR(64) NOT NULL, -- cdm_cluster_subnet.network_cidr
    cluster_subnet_dhcp_enabled BOOLEAN NOT NULL, -- cdm_cluster_subnet.dhcp_enabled
    cluster_subnet_gateway_enabled BOOLEAN NOT NULL, -- cdm_cluster_subnet.gateway_enabled
    cluster_subnet_gateway_ip_address VARCHAR(40), -- cdm_cluster_subnet.gateway_ip_address
    cluster_subnet_ipv6_address_mode_code VARCHAR(100), -- cdm_cluster_subnet.ipv6_address_mode_code
    cluster_subnet_ipv6_ra_mode_code VARCHAR(100), -- cdm_cluster_subnet.ipv6_ra_mode_code
    cluster_ip_address VARCHAR(40) NOT NULL,
    protection_flag BOOLEAN NOT NULL,
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_router_id, routing_interface_seq),
    CONSTRAINT cdm_disaster_recovery_result_external_routing_interface_router_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_router_id) REFERENCES cdm_disaster_recovery_result_router (recovery_result_id, protection_cluster_tenant_id, protection_cluster_router_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_extra_route
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_extra_route (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_router_id INTEGER NOT NULL, -- cdm_cluster_router.id
    extra_route_seq INTEGER NOT NULL,
    protection_cluster_destination VARCHAR(64) NOT NULL, -- cdm_cluster_router_extra_route.destination
    protection_cluster_nexthop VARCHAR(40) NOT NULL, -- cdm_cluster_router_extra_route.nexthop
    recovery_cluster_destination VARCHAR(64), -- cdm_cluster_router_extra_route.destination
    recovery_cluster_nexthop VARCHAR(40), -- cdm_cluster_router_extra_route.nexthop
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_router_id, extra_route_seq),
    CONSTRAINT cdm_disaster_recovery_result_extra_route_router_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_router_id) REFERENCES cdm_disaster_recovery_result_router (recovery_result_id, protection_cluster_tenant_id, protection_cluster_router_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_security_group
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_security_group (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_security_group_id INTEGER NOT NULL, -- cdm_cluster_security_group.id
    protection_cluster_security_group_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_security_group.uuid
    protection_cluster_security_group_name VARCHAR(255) NOT NULL, -- cdm_cluster_security_group.name
    protection_cluster_security_group_description VARCHAR(255), -- cdm_cluster_security_group.description
    recovery_cluster_security_group_id INTEGER, -- cdm_cluster_security_group.id
    recovery_cluster_security_group_uuid VARCHAR(36), -- cdm_cluster_security_group.uuid
    recovery_cluster_security_group_name VARCHAR(255), -- cdm_cluster_security_group.name
    recovery_cluster_security_group_description VARCHAR(255), -- cdm_cluster_security_group.description
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_security_group_id),
    CONSTRAINT cdm_disaster_recovery_result_security_group_tenant_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id) REFERENCES cdm_disaster_recovery_result_tenant (recovery_result_id, protection_cluster_tenant_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_security_group_rule
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_security_group_rule (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_security_group_id INTEGER NOT NULL, -- cdm_cluster_security_group.id
    protection_cluster_security_group_rule_id INTEGER NOT NULL, -- cdm_cluster_security_group_rule.id
    protection_cluster_security_group_rule_remote_security_group_id INTEGER, -- cdm_cluster_security_group_rule.remote_security_group_id
    protection_cluster_security_group_rule_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_security_group_rule.rule_uuid
    protection_cluster_security_group_rule_description VARCHAR(255), -- cdm_cluster_security_group_rule.description
    protection_cluster_security_group_rule_network_cidr VARCHAR(64), -- cdm_cluster_security_group_rule.network_cidr
    protection_cluster_security_group_rule_direction VARCHAR(20) NOT NULL, -- cdm_cluster_security_group_rule.direction
    protection_cluster_security_group_rule_port_range_min INTEGER, -- cdm_cluster_security_group_rule.port_range_min
    protection_cluster_security_group_rule_port_range_max INTEGER, -- cdm_cluster_security_group_rule.port_range_max
    protection_cluster_security_group_rule_protocol VARCHAR(20), -- cdm_cluster_security_group_rule.protocol
    protection_cluster_security_group_rule_ether_type INTEGER NOT NULL, -- cdm_cluster_security_group_rule.ether_type
    recovery_cluster_security_group_rule_id INTEGER, -- cdm_cluster_security_group_rule.id
    recovery_cluster_security_group_rule_remote_security_group_id INTEGER,  -- cdm_cluster_security_group_rule.remote_security_group_id
    recovery_cluster_security_group_rule_uuid VARCHAR(36), -- cdm_cluster_security_group_rule.rule_uuid
    recovery_cluster_security_group_rule_description VARCHAR(255), -- cdm_cluster_security_group_rule.description
    recovery_cluster_security_group_rule_network_cidr VARCHAR(64), -- cdm_cluster_security_group_rule.network_cidr
    recovery_cluster_security_group_rule_direction VARCHAR(20), -- cdm_cluster_security_group_rule.direction
    recovery_cluster_security_group_rule_port_range_min INTEGER, -- cdm_cluster_security_group_rule.port_range_min
    recovery_cluster_security_group_rule_port_range_max INTEGER, -- cdm_cluster_security_group_rule.port_range_max
    recovery_cluster_security_group_rule_protocol VARCHAR(20), -- cdm_cluster_security_group_rule.protocol
    recovery_cluster_security_group_rule_ether_type INTEGER, -- cdm_cluster_security_group_rule.ether_type
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_security_group_id, protection_cluster_security_group_rule_id),
    CONSTRAINT cdm_disaster_recovery_result_security_group_rule_security_group_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_security_group_id) REFERENCES cdm_disaster_recovery_result_security_group (recovery_result_id, protection_cluster_tenant_id, protection_cluster_security_group_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT cdm_disaster_recovery_result_security_group_rule_remote_security_group_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_security_group_rule_remote_security_group_id) REFERENCES cdm_disaster_recovery_result_security_group (recovery_result_id, protection_cluster_tenant_id, protection_cluster_security_group_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_volume
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_volume (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_volume_id INTEGER NOT NULL, -- cdm_cluster_volume.id
    protection_cluster_volume_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_volume.uuid
    protection_cluster_volume_name VARCHAR(255), -- cdm_cluster_volume.name
    protection_cluster_volume_description VARCHAR(255), -- cdm_cluster_volume.description
    protection_cluster_volume_size_bytes INTEGER NOT NULL, -- cdm_cluster_volume.size_bytes
    protection_cluster_volume_multiattach BOOL NOT NULL, -- cdm_cluster_volume.multiattach
    protection_cluster_volume_bootable BOOL NOT NULL, -- cdm_cluster_volume.bootable
    protection_cluster_volume_readonly BOOL NOT NULL, -- cdm_cluster_volume.readonly
    protection_cluster_storage_id INTEGER NOT NULL, -- cdm_cluster_storage.id
    protection_cluster_storage_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_storage.uuid
    protection_cluster_storage_name VARCHAR(255) NOT NULL, -- cdm_cluster_storage.name
    protection_cluster_storage_description VARCHAR(255), -- cdm_cluster_storage.description
    protection_cluster_storage_type_code VARCHAR(100) NOT NULL, -- cdm_cluster_storage.type_code
    recovery_cluster_volume_id INTEGER, -- cdm_cluster_volume.id
    recovery_cluster_volume_uuid VARCHAR(36), -- cdm_cluster_volume.uuid
    recovery_cluster_volume_name VARCHAR(255), -- cdm_cluster_volume.name
    recovery_cluster_volume_description VARCHAR(255), -- cdm_cluster_volume.description
    recovery_cluster_volume_size_bytes INTEGER, -- cdm_cluster_volume.size_bytes
    recovery_cluster_volume_multiattach BOOLEAN, -- cdm_cluster_volume.multiattach
    recovery_cluster_volume_bootable BOOLEAN, -- cdm_cluster_volume.bootable
    recovery_cluster_volume_readonly BOOLEAN, -- cdm_cluster_volume.readonly
    recovery_cluster_storage_id INTEGER, -- cdm_cluster_storage.id
    recovery_cluster_storage_uuid VARCHAR(36), -- cdm_cluster_storage.uuid
    recovery_cluster_storage_name VARCHAR(255), -- cdm_cluster_storage.name
    recovery_cluster_storage_description VARCHAR(255), -- cdm_cluster_storage.description
    recovery_cluster_storage_type_code VARCHAR(100), -- cdm_cluster_storage.type_code
    recovery_point_type_code VARCHAR(100) NOT NULL,
    recovery_point INTEGER NOT NULL,
    started_at INTEGER,
    finished_at INTEGER,
    result_code VARCHAR(100) NOT NULL,
    failed_reason_code VARCHAR(100),
    failed_reason_contents TEXT,
    rollback_flag BOOL NOT NULL,
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_volume_id),
    CONSTRAINT cdm_disaster_recovery_result_volume_tenant_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id) REFERENCES cdm_disaster_recovery_result_tenant (recovery_result_id, protection_cluster_tenant_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_instance
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_instance (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_instance_id INTEGER NOT NULL, -- cdm_cluster_instance.id
    protection_cluster_instance_uuid VARCHAR(36) NOT NULL, -- cdm_cluster_instance.uuid
    protection_cluster_instance_name VARCHAR(255) NOT NULL, -- cdm_cluster_instance.name
    protection_cluster_instance_description VARCHAR(255),  -- cdm_cluster_instance.description
    protection_cluster_instance_spec_id INTEGER NOT NULL, -- cdm_cluster_instance_spec.id
    protection_cluster_keypair_id INTEGER, -- cdm_cluster_keypair.id
    protection_cluster_availability_zone_id INTEGER NOT NULL, -- cdm_cluster_availability_zone.id
    protection_cluster_availability_zone_name VARCHAR(255) NOT NULL, -- cdm_cluster_availability_zone.name
    protection_cluster_hypervisor_id INTEGER NOT NULL, -- cdm_cluster_hypervisor.id
    protection_cluster_hypervisor_type_code VARCHAR(100) NOT NULL, -- cdm_cluster_hypervisor.type_code
    protection_cluster_hypervisor_hostname VARCHAR(255) NOT NULL, -- cdm_cluster_hypervisor.hostname
    protection_cluster_hypervisor_ip_address VARCHAR(40) NOT NULL, -- cdm_cluster_hypervisor.ip_address
    recovery_cluster_instance_id INTEGER, -- cdm_cluster_instance.id
    recovery_cluster_instance_uuid VARCHAR(36), -- cdm_cluster_instance.uuid
    recovery_cluster_instance_name VARCHAR(255), -- cdm_cluster_instance.name
    recovery_cluster_instance_description VARCHAR(255), -- cdm_cluster_instance.description
    recovery_cluster_availability_zone_id INTEGER, -- cdm_cluster_availability_zone.id
    recovery_cluster_availability_zone_name VARCHAR(255), -- cdm_cluster_availability_zone.name
    recovery_cluster_hypervisor_id INTEGER, -- cdm_cluster_hypervisor.id
    recovery_cluster_hypervisor_type_code VARCHAR(100), -- cdm_cluster_hypervisor.type_code
    recovery_cluster_hypervisor_hostname VARCHAR(255), -- cdm_cluster_hypervisor.hostname
    recovery_cluster_hypervisor_ip_address VARCHAR(40), -- cdm_cluster_hypervisor.ip_address
    recovery_point_type_code VARCHAR(100) NOT NULL,
    recovery_point INTEGER NOT NULL,
    auto_start_flag BOOLEAN NOT NULL,
    diagnosis_flag BOOLEAN NOT NULL,
    diagnosis_method_code VARCHAR(100),
    diagnosis_method_data VARCHAR(300),
    diagnosis_timeout INTEGER,
    started_at INTEGER,
    finished_at INTEGER,
    result_code VARCHAR(100) NOT NULL,
    failed_reason_code VARCHAR(100),
    failed_reason_contents TEXT,
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id),
    CONSTRAINT cdm_disaster_recovery_result_instance_tenant_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id) REFERENCES cdm_disaster_recovery_result_tenant (recovery_result_id, protection_cluster_tenant_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT cdm_disaster_recovery_result_instance_instance_spec_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_instance_spec_id) REFERENCES cdm_disaster_recovery_result_instance_spec (recovery_result_id, protection_cluster_instance_spec_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT cdm_disaster_recovery_result_instance_keypair_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_keypair_id) REFERENCES cdm_disaster_recovery_result_keypair (recovery_result_id, protection_cluster_keypair_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_instance_dependency
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_instance_dependency (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_instance_id INTEGER NOT NULL, -- cdm_cluster_instance.id
    dependency_protection_cluster_instance_id INTEGER NOT NULL, -- cdm_cluster_instance.id
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id, dependency_protection_cluster_instance_id),
    CONSTRAINT cdm_disaster_recovery_result_instance_dependency_instance_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id) REFERENCES cdm_disaster_recovery_result_instance (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_instance_network
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_instance_network (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_instance_id INTEGER NOT NULL, -- cdm_cluster_instance.id
    instance_network_seq INTEGER NOT NULL,
    protection_cluster_network_id INTEGER NOT NULL, -- cdm_cluster_network.id
    protection_cluster_subnet_id INTEGER NOT NULL, -- cdm_cluster_subnet.id
    protection_cluster_floating_ip_id INT, -- cdm_cluster_floating_ip.id
    protection_cluster_dhcp_flag BOOLEAN NOT NULL,
    protection_cluster_ip_address VARCHAR(40) NOT NULL,
    recovery_cluster_dhcp_flag BOOLEAN,
    recovery_cluster_ip_address VARCHAR(40),
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id, instance_network_seq),
    CONSTRAINT cdm_disaster_recovery_result_instance_network_instance_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id) REFERENCES cdm_disaster_recovery_result_instance (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT cdm_disaster_recovery_result_instance_network_subnet_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_network_id, protection_cluster_subnet_id) REFERENCES cdm_disaster_recovery_result_subnet (recovery_result_id, protection_cluster_tenant_id, protection_cluster_network_id, protection_cluster_subnet_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_instance_security_group
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_instance_security_group (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_instance_id INTEGER NOT NULL, -- cdm_cluster_instance.id
    protection_cluster_security_group_id INTEGER NOT NULL, -- cdm_cluster_security_group.id
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id, protection_cluster_security_group_id),
    CONSTRAINT cdm_disaster_recovery_result_instance_security_group_instance_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id) REFERENCES cdm_disaster_recovery_result_instance (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT cdm_disaster_recovery_result_instance_security_group_security_group_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_security_group_id) REFERENCES cdm_disaster_recovery_result_security_group (recovery_result_id, protection_cluster_tenant_id, protection_cluster_security_group_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_result_instance_volume
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_result_instance_volume (
    recovery_result_id INTEGER NOT NULL,
    protection_cluster_tenant_id INTEGER NOT NULL, -- cdm_cluster_tenant.id
    protection_cluster_instance_id INTEGER NOT NULL, -- cdm_cluster_instance.id
    protection_cluster_volume_id INTEGER NOT NULL, -- cdm_cluster_volume.id
    protection_cluster_device_path VARCHAR(4096) NOT NULL,
    protection_cluster_boot_index INTEGER NOT NULL,
    recovery_cluster_device_path VARCHAR(4096),
    recovery_cluster_boot_index INTEGER,
    PRIMARY KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id, protection_cluster_volume_id),
    CONSTRAINT cdm_disaster_recovery_result_instance_volume_instance_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id) REFERENCES cdm_disaster_recovery_result_instance (recovery_result_id, protection_cluster_tenant_id, protection_cluster_instance_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT cdm_disaster_recovery_result_instance_volume_volume_result_id_fk FOREIGN KEY (recovery_result_id, protection_cluster_tenant_id, protection_cluster_volume_id) REFERENCES cdm_disaster_recovery_result_volume (recovery_result_id, protection_cluster_tenant_id, protection_cluster_volume_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_instance_template
CREATE SEQUENCE IF NOT EXISTS cdm_disaster_recovery_instance_template_seq;
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_instance_template (
    id INTEGER NOT NULL DEFAULT NEXTVAL('cdm_disaster_recovery_instance_template_seq'),
    owner_group_id INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    remarks VARCHAR(300),
    created_at INTEGER NOT NULL,
    PRIMARY KEY (id)
);

-- cdm_disaster_recovery_instance_template_instance
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_instance_template_instance (
    instance_template_id INTEGER NOT NULL,
    protection_cluster_instance_name VARCHAR(255) NOT NULL, -- cdm_cluster_instance.name
    auto_start_flag BOOLEAN DEFAULT false,
    diagnosis_flag BOOLEAN DEFAULT false,
    diagnosis_method_code VARCHAR(100),
    diagnosis_method_data VARCHAR(300),
    diagnosis_timeout INTEGER ,
    PRIMARY KEY (instance_template_id, protection_cluster_instance_name),
    CONSTRAINT cdm_disaster_recovery_plan_instance_instance_template_id_fk FOREIGN KEY (instance_template_id) REFERENCES cdm_disaster_recovery_instance_template (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

-- cdm_disaster_recovery_instance_template_instance_dependency
CREATE TABLE IF NOT EXISTS cdm_disaster_recovery_instance_template_instance_dependency (
    instance_template_id INTEGER NOT NULL,
    protection_cluster_instance_name VARCHAR(255) NOT NULL, -- cdm_cluster_instance.name
    depend_protection_cluster_instance_name VARCHAR(255) NOT NULL, -- cdm_cluster_instance.name
    PRIMARY KEY (instance_template_id, protection_cluster_instance_name, depend_protection_cluster_instance_name),
    CONSTRAINT cdm_disaster_recovery_instance_tempalte_instance_dependency_instance_template_id_fk FOREIGN KEY (instance_template_id) REFERENCES cdm_disaster_recovery_instance_template (id) ON UPDATE RESTRICT ON DELETE RESTRICT
);