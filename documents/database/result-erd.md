```plantuml
@startuml
left to right direction
skinparam linetype ortho
hide circle

entity cdm_disaster_recovery_result {
  id : INT <<generated>>
  --
  owner_group_id : INT <<FK>> <<NN>>
  operator_id : INT <<NN>>
  operator_account : VARCHAR(30) <<NN>>
  operator_name : VARCHAR(255) <<NN>>
  operator_department : VARCHAR(255)
  operator_position : VARCHAR(255)
  approver_id : INT
  approver_account : VARCHAR(30)
  approver_name : VARCHAR(255)
  approver_department : VARCHAR(255)
  approver_position : VARCHAR(255)
  protection_group_id : INT <<NN>>
  protection_group_name : VARCHAR(255) <<NN>>
  protection_group_remarks : VARCHAR(300)
  protection_cluster_id : INT <<NN>>
  protection_cluster_type_code : VARCHAR(100) <<NN>>
  protection_cluster_name : VARCHAR(255) <<NN>>
  protection_cluster_remarks : VARCHAR(300)
  recovery_plan_id : INT <<NN>>
  recovery_plan_name : VARCHAR(255) <<NN>>
  recovery_plan_remarks : VARCHAR(300)
  recovery_cluster_id : INT <<NN>>
  recovery_cluster_type_code : VARCHAR(100) <<NN>>
  recovery_cluster_name : VARCHAR(255) <<NN>>
  recovery_cluster_remarks : VARCHAR(300)
  recovery_type_code : VARCHAR(100) <<NN>>
  recovery_direction_code : VARCHAR(100) <<NN>>
  recovery_point_objective_type : VARCHAR(100) <<NN>>
  recovery_point_objective : INT <<NN>>
  recovery_time_objective : INT <<NN>>
  recovery_point_type_code : VARCHAR(100) <<NN>>
  recovery_point : INT <<NN>>
  schedule_type : VARCHAR(100)
  started_at : INT <<NN>>
  finished_at : INT <<NN>>
  elapsed_time : INT <<NN>>
  result_code : VARCHAR(100) <<NN>>
  warning_flag : BOOL <<NN>>
}

entity cdm_disaster_recovery_result_raw {
  recovery_result_id : INT <<FK>> <<NN>>
  --
  recovery_job_id : INT <<NN>>
  contents : TEXT
  shared_task : TEXT
}

entity cdm_disaster_recovery_result_warning_reason {
  recovery_result_id : INT <<FK>> <<NN>>
  reason_seq : INT <<generated>>
  --
  code : VARCHAR(100) <<NN>>
  contents : TEXT
}

entity cdm_disaster_recovery_result_failed_reason {
  recovery_result_id : INT <<FK>> <<NN>>
  reason_seq : INT <<generated>>
  --
  code : VARCHAR(100) <<NN>>
  contents : TEXT
}

entity cdm_disaster_recovery_result_task_log {
  recovery_result_id : INT <<FK>> <<NN>>
  log_seq : INT <<generated>>
  --
  code : VARCHAR(100) <<NN>>
  contents : TEXT
  log_dt : INT <<NN>>
}

entity cdm_disaster_recovery_result_tenant {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<NN>>
  --
  protection_cluster_tenant_uuid : VARCHAR(36) <<NN>>
  protection_cluster_tenant_name : VARCHAR(255) <<NN>>
  protection_cluster_tenant_description : TEXT
  protection_cluster_tenant_enabled : BOOL <<NN>>
  recovery_cluster_tenant_id : INT
  recovery_cluster_tenant_uuid : VARCHAR(36)
  recovery_cluster_tenant_name : VARCHAR(255)
  recovery_cluster_tenant_description : TEXT
  recovery_cluster_tenant_enabled : BOOL
}

entity cdm_disaster_recovery_result_instance_spec {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_instance_spec_id : INT <<NN>>
  --
  protection_cluster_instance_spec_uuid : VARCHAR(255) <<NN>>
  protection_cluster_instance_spec_name : VARCHAR(255) <<NN>>
  protection_cluster_instance_spec_description : VARCHAR(255)
  protection_cluster_instance_spec_vcpu_total_cnt : INT <<NN>>
  protection_cluster_instance_spec_mem_total_bytes : INT <<NN>>
  protection_cluster_instance_spec_disk_total_bytes : INT <<NN>>
  protection_cluster_instance_spec_swap_total_bytes : INT <<NN>>
  protection_cluster_instance_spec_ephemeral_total_bytes : INT <<NN>>
  recovery_cluster_instance_spec_id : INT
  recovery_cluster_instance_spec_uuid : VARCHAR(255)
  recovery_cluster_instance_spec_name : VARCHAR(255)
  recovery_cluster_instance_spec_description : VARCHAR(255)
  recovery_cluster_instance_spec_vcpu_total_cnt : INT
  recovery_cluster_instance_spec_mem_total_bytes : INT
  recovery_cluster_instance_spec_disk_total_bytes : INT
  recovery_cluster_instance_spec_swap_total_bytes : INT
  recovery_cluster_instance_spec_ephemeral_total_bytes : INT
}

entity cdm_disaster_recovery_result_instance_extra_spec {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_instance_spec_id : INT <<FK>> <<NN>>
  protection_cluster_instance_extra_spec_id : INT <<NN>>
  --
  protection_cluster_instance_extra_spec_key : VARCHAR(255) <<NN>>
  protection_cluster_instance_extra_spec_value : VARCHAR(255) <<NN>>
  recovery_cluster_instance_extra_spec_id : INT
  recovery_cluster_instance_extra_spec_key : VARCHAR(255)
  recovery_cluster_instance_extra_spec_value : VARCHAR(255)
}

entity cdm_disaster_recovery_result_keypair {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_keypair_id : INT <<NN>>
  --
  protection_cluster_keypair_name : VARCHAR(255) <<NN>>
  protection_cluster_keypair_fingerprint : VARCHAR(100) <<NN>>
  protection_cluster_keypair_public_key : VARCHAR(2048) <<NN>>
  protection_cluster_keypair_type_code : VARCHAR(50) <<NN>>
  recovery_cluster_keypair_id : INT
  recovery_cluster_keypair_name : VARCHAR(255)
  recovery_cluster_keypair_fingerprint : VARCHAR(100)
  recovery_cluster_keypair_public_key : VARCHAR(2048)
  recovery_cluster_keypair_type_code : VARCHAR(50)
}

entity cdm_disaster_recovery_result_network {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_network_id : INT <<NN>>
  --
  protection_cluster_network_uuid : VARCHAR(36) <<NN>>
  protection_cluster_network_name : VARCHAR(255) <<NN>>
  protection_cluster_network_description : VARCHAR(255)
  protection_cluster_network_type_code : VARCHAR(100) <<NN>>
  protection_cluster_network_state : VARCHAR(20) <<NN>>
  recovery_cluster_network_id : INT
  recovery_cluster_network_uuid : VARCHAR(36)
  recovery_cluster_network_name : VARCHAR(255)
  recovery_cluster_network_description : VARCHAR(255)
  recovery_cluster_network_type_code : VARCHAR(100)
  recovery_cluster_network_state : VARCHAR(20)
}

entity cdm_disaster_recovery_result_subnet {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_network_id : INT <<FK>> <<NN>>
  protection_cluster_subnet_id : INT <<NN>>
  --
  protection_cluster_subnet_uuid : VARCHAR(36) <<NN>>
  protection_cluster_subnet_name : VARCHAR(255) <<NN>>
  protection_cluster_subnet_description : VARCHAR(255)
  protection_cluster_subnet_network_cidr : VARCHAR(64) <<NN>>
  protection_cluster_subnet_dhcp_enabled : BOOL <<NN>>
  protection_cluster_subnet_gateway_enabled : BOOL <<NN>>
  protection_cluster_subnet_gateway_ip_address : VARCHAR(40)
  protection_cluster_subnet_ipv6_address_mode_code : VARCHAR(100)
  protection_cluster_subnet_ipv6_ra_mode_code : VARCHAR(100)
  recovery_cluster_subnet_id : INT
  recovery_cluster_subnet_uuid : VARCHAR(36)
  recovery_cluster_subnet_name : VARCHAR(255)
  recovery_cluster_subnet_description : VARCHAR(255)
  recovery_cluster_subnet_network_cidr : VARCHAR(64)
  recovery_cluster_subnet_dhcp_enabled : BOOL
  recovery_cluster_subnet_gateway_enabled : BOOL
  recovery_cluster_subnet_gateway_ip_address : VARCHAR(40)
  recovery_cluster_subnet_ipv6_address_mode_code : VARCHAR(100)
  recovery_cluster_subnet_ipv6_ra_mode_code : VARCHAR(100)
}

entity cdm_disaster_recovery_result_subnet_dhcp_pool {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_network_id : INT <<FK>> <<NN>>
  protection_cluster_subnet_id : INT <<FK>> <<NN>>
  dhcp_pool_seq : INT <<generated>>
  --
  protection_cluster_start_ip_address : VARCHAR(40) <<NN>>
  protection_cluster_end_ip_address : VARCHAR(40) <<NN>>
  recovery_cluster_start_ip_address : VARCHAR(40)
  recovery_cluster_end_ip_address : VARCHAR(40)
}

entity cdm_disaster_recovery_result_subnet_nameserver {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_network_id : INT <<FK>> <<NN>>
  protection_cluster_subnet_id : INT <<FK>> <<NN>>
  nameserver_seq : INT <<generated>>
  --
  protection_cluster_nameserver : VARCHAR(255) <<NN>>
  recovery_cluster_nameserver : VARCHAR(255)
}

entity cdm_disaster_recovery_result_floating_ip {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_floating_ip_id : INT <<NN>>
  --
  protection_cluster_floating_ip_uuid : VARCHAR(36) <<NN>>
  protection_cluster_floating_ip_description : VARCHAR(255)
  protection_cluster_floating_ip_ip_address : VARCHAR(40) <<NN>>
  protection_cluster_network_id : INT <<NN>>
  protection_cluster_network_uuid : VARCHAR(36) <<NN>>
  protection_cluster_network_name : VARCHAR(255) <<NN>>
  protection_cluster_network_description : VARCHAR(255)
  protection_cluster_network_type_code : VARCHAR(100) <<NN>>
  protection_cluster_network_state : VARCHAR(20) <<NN>>
  recovery_cluster_floating_ip_id : INT
  recovery_cluster_floating_ip_uuid : VARCHAR(36)
  recovery_cluster_floating_ip_description : VARCHAR(255)
  recovery_cluster_floating_ip_ip_address : VARCHAR(40)
  recovery_cluster_network_id : INT
  recovery_cluster_network_uuid : VARCHAR(36)
  recovery_cluster_network_name : VARCHAR(255)
  recovery_cluster_network_description : VARCHAR(255)
  recovery_cluster_network_type_code : VARCHAR(100)
  recovery_cluster_network_state : VARCHAR(20)
}

entity cdm_disaster_recovery_result_router {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_router_id : INT <<NN>>
  --
  protection_cluster_router_uuid : VARCHAR(36) <<NN>>
  protection_cluster_router_name : VARCHAR(255) <<NN>>
  protection_cluster_router_description : VARCHAR(255)
  protection_cluster_router_state : VARCHAR(20) <<NN>>
  recovery_cluster_router_id : INT
  recovery_cluster_router_uuid : VARCHAR(36)
  recovery_cluster_router_name : VARCHAR(255)
  recovery_cluster_router_description : VARCHAR(255)
  recovery_cluster_router_state : VARCHAR(20)
}

entity cdm_disaster_recovery_result_internal_routing_interface {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_router_id : INT <<FK>> <<NN>>
  routing_interface_seq : INT <<generated>>
  --
  protection_cluster_network_id : INT <<NN>>
  protection_cluster_subnet_id : INT <<NN>>
  protection_cluster_ip_address : VARCHAR(40) <<NN>>
  recovery_cluster_network_id : INT
  recovery_cluster_subnet_id : INT
  recovery_cluster_ip_address : VARCHAR(40)
}

entity cdm_disaster_recovery_result_external_routing_interface {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_router_id : INT <<FK>> <<NN>>
  routing_interface_seq : INT <<generated>>
  --
  cluster_network_id : INT <<NN>>
  cluster_network_uuid : VARCHAR(36) <<NN>>
  cluster_network_name : VARCHAR(255) <<NN>>
  cluster_network_description : VARCHAR(255)
  cluster_network_type_code : VARCHAR(100) <<NN>>
  cluster_network_state : VARCHAR(20) <<NN>>
  cluster_subnet_id : INT <<NN>>
  cluster_subnet_uuid : VARCHAR(36) <<NN>>
  cluster_subnet_name : VARCHAR(255) <<NN>>
  cluster_subnet_description : VARCHAR(255)
  cluster_subnet_network_cidr : VARCHAR(64) <<NN>>
  cluster_subnet_dhcp_enabled : BOOL <<NN>>
  cluster_subnet_gateway_enabled : BOOL <<NN>>
  cluster_subnet_gateway_ip_address : VARCHAR(40)
  cluster_subnet_ipv6_address_mode_code : VARCHAR(100)
  cluster_subnet_ipv6_ra_mode_code : VARCHAR(100)
  cluster_ip_address : VARCHAR(40) <<NN>>
  protection_flag : BOOL <<NN>>
}

entity cdm_disaster_recovery_result_extra_route {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_router_id : INT <<FK>> <<NN>>
  extra_route_seq : INT <<generated>>
  --
  protection_cluster_destination : VARCHAR(64) <<NN>>
  protection_cluster_nexthop : VARCHAR(40) <<NN>>
  recovery_cluster_destination : VARCHAR(64)
  recovery_cluster_nexthop : VARCHAR(40)
}

entity cdm_disaster_recovery_result_security_group {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_security_group_id : INT <<NN>>
  --
  protection_cluster_security_group_uuid : VARCHAR(36) <<NN>>
  protection_cluster_security_group_name : VARCHAR(255) <<NN>>
  protection_cluster_security_group_description : VARCHAR(255)
  recovery_cluster_security_group_id : INT
  recovery_cluster_security_group_uuid : VARCHAR(36)
  recovery_cluster_security_group_name : VARCHAR(255)
  recovery_cluster_security_group_description : VARCHAR(255)
}

entity cdm_disaster_recovery_result_security_group_rule {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_security_group_id : INT <<FK>> <<NN>>
  protection_cluster_security_group_rule_id : INT <<NN>>
  --
  protection_cluster_security_group_rule_remote_security_group_id : INT <<FK>>
  protection_cluster_security_group_rule_uuid : VARCHAR(36) <<NN>>
  protection_cluster_security_group_rule_description : VARCHAR(255)
  protection_cluster_security_group_rule_network_cidr : VARCHAR(64)
  protection_cluster_security_group_rule_direction : VARCHAR(20) <<NN>>
  protection_cluster_security_group_rule_port_range_min : INT
  protection_cluster_security_group_rule_port_range_max : INT
  protection_cluster_security_group_rule_protocol : VARCHAR(20)
  protection_cluster_security_group_rule_ether_type : INT <<NN>>
  recovery_cluster_security_group_rule_id : INT
  recovery_cluster_security_group_rule_remote_security_group_id : INT
  recovery_cluster_security_group_rule_uuid : VARCHAR(36)
  recovery_cluster_security_group_rule_description : VARCHAR(255)
  recovery_cluster_security_group_rule_network_cidr : VARCHAR(64)
  recovery_cluster_security_group_rule_direction : VARCHAR(20)
  recovery_cluster_security_group_rule_port_range_min : INT
  recovery_cluster_security_group_rule_port_range_max : INT
  recovery_cluster_security_group_rule_protocol : VARCHAR(20)
  recovery_cluster_security_group_rule_ether_type : INT
}

entity cdm_disaster_recovery_result_instance {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_instance_id : INT <<NN>>
  --
  protection_cluster_instance_uuid : VARCHAR(36) <<NN>>
  protection_cluster_instance_name : VARCHAR(255) <<NN>>
  protection_cluster_instance_description : VARCHAR(255)
  protection_cluster_instance_spec_id : INT <<FK>> <<NN>>
  protection_cluster_keypair_id : INT <<FK>>
  protection_cluster_availability_zone_id : INT <<NN>>
  protection_cluster_availability_zone_name : VARCHAR(255) <<NN>>
  protection_cluster_hypervisor_id : INT <<NN>>
  protection_cluster_hypervisor_type_code : VARCHAR(100) <<NN>>
  protection_cluster_hypervisor_hostname : VARCHAR(255) <<NN>>
  protection_cluster_hypervisor_ip_address : VARCHAR(40) <<NN>>
  recovery_cluster_instance_id : INT
  recovery_cluster_instance_uuid : VARCHAR(36)
  recovery_cluster_instance_name : VARCHAR(255)
  recovery_cluster_instance_description : VARCHAR(255)
  recovery_cluster_availability_zone_id : INT
  recovery_cluster_availability_zone_name : VARCHAR(255)
  recovery_cluster_hypervisor_id : INT
  recovery_cluster_hypervisor_type_code : VARCHAR(100)
  recovery_cluster_hypervisor_hostname : VARCHAR(255)
  recovery_cluster_hypervisor_ip_address : VARCHAR(40)
  recovery_point_type_code : VARCHAR(100) <<NN>>
  recovery_point : INT <<NN>>
  auto_start_flag : BOOL <<NN>>
  diagnosis_flag : BOOL <<NN>>
  diagnosis_method_code : VARCHAR(100)
  diagnosis_method_data : VARCHAR(300)
  diagnosis_timeout : INT
  started_at : INT
  finished_at : INT
  result_code : VARCHAR(100) <<NN>>
  failed_reason_code : VARCHAR(100)
  failed_reason_contents : TEXT
}

entity cdm_disaster_recovery_result_instance_dependency {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_instance_id : INT <<FK>> <<NN>>
  dependency_protection_cluster_instance_id : INT <<NN>>
  --
}

entity cdm_disaster_recovery_result_instance_network {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_instance_id : INT <<FK>> <<NN>>
  instance_network_seq : INT <<generated>>
  --
  protection_cluster_network_id : INT <<FK>> <<NN>>
  protection_cluster_subnet_id : INT <<FK>> <<NN>>
  protection_cluster_floating_ip_id : INT
  protection_cluster_dhcp_flag : BOOL <<NN>>
  protection_cluster_ip_address : VARCHAR(40) <<NN>>
  recovery_cluster_dhcp_flag : BOOL
  recovery_cluster_ip_address : VARCHAR(40)
}

entity cdm_disaster_recovery_result_instance_security_group {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_instance_id : INT <<FK>> <<NN>>
  protection_cluster_security_group_id : INT <<FK>> <<NN>>
  --
}

entity cdm_disaster_recovery_result_volume {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_volume_id : INT <<NN>>
  --
  protection_cluster_volume_uuid : VARCHAR(36) <<NN>>
  protection_cluster_volume_name : VARCHAR(255)
  protection_cluster_volume_description : VARCHAR(255)
  protection_cluster_volume_size_bytes : INT <<NN>>
  protection_cluster_volume_multiattach : BOOL <<NN>>
  protection_cluster_volume_bootable : BOOL <<NN>>
  protection_cluster_volume_readonly : BOOL <<NN>>
  protection_cluster_storage_id : INT <<NN>>
  protection_cluster_storage_uuid : VARCHAR(36) <<NN>>
  protection_cluster_storage_name : VARCHAR(255) <<NN>>
  protection_cluster_storage_description : VARCHAR(255)
  protection_cluster_storage_type_code : VARCHAR(100) <<NN>>
  recovery_cluster_volume_id : INT
  recovery_cluster_volume_uuid : VARCHAR(36)
  recovery_cluster_volume_name : VARCHAR(255)
  recovery_cluster_volume_description : VARCHAR(255)
  recovery_cluster_volume_size_bytes : INT
  recovery_cluster_volume_multiattach : BOOL
  recovery_cluster_volume_bootable : BOOL
  recovery_cluster_volume_readonly : BOOL
  recovery_cluster_storage_id : INT
  recovery_cluster_storage_uuid : VARCHAR(36)
  recovery_cluster_storage_name : VARCHAR(255)
  recovery_cluster_storage_description : VARCHAR(255)
  recovery_cluster_storage_type_code : VARCHAR(100)
  recovery_point_type_code : VARCHAR(100) <<NN>>
  recovery_point : INT <<NN>>
  started_at : INT
  finished_at : INT
  result_code : VARCHAR(100) <<NN>>
  failed_reason_code : VARCHAR(100)
  failed_reason_contents : TEXT
  rollback_flag : BOOL <<NN>>
}

entity cdm_disaster_recovery_result_instance_volume {
  recovery_result_id : INT <<FK>> <<NN>>
  protection_cluster_tenant_id : INT <<FK>> <<NN>>
  protection_cluster_instance_id : INT <<FK>> <<NN>>
  protection_cluster_volume_id : INT <<FK>> <<NN>>
  --
  protection_cluster_device_path : VARCHAR(4096) <<NN>>
  protection_cluster_boot_index : INT <<NN>>
  recovery_cluster_device_path : VARCHAR(4096)
  recovery_cluster_boot_index : INT
}

cdm_disaster_recovery_result ||-u-o{ cdm_disaster_recovery_result_raw
cdm_disaster_recovery_result ||-u-o{ cdm_disaster_recovery_result_warning_reason
cdm_disaster_recovery_result ||-u-o{ cdm_disaster_recovery_result_failed_reason
cdm_disaster_recovery_result ||-u-o{ cdm_disaster_recovery_result_task_log
cdm_disaster_recovery_result ||--o{ cdm_disaster_recovery_result_tenant
cdm_disaster_recovery_result ||-r-o{ cdm_disaster_recovery_result_instance_spec
cdm_disaster_recovery_result ||-r-o{ cdm_disaster_recovery_result_keypair
cdm_disaster_recovery_result ||-l-o{ cdm_disaster_recovery_result_floating_ip
cdm_disaster_recovery_result_tenant ||--o{ cdm_disaster_recovery_result_network
cdm_disaster_recovery_result_tenant ||-l-o{ cdm_disaster_recovery_result_router
cdm_disaster_recovery_result_tenant ||--o{ cdm_disaster_recovery_result_security_group
cdm_disaster_recovery_result_tenant ||-r-o{ cdm_disaster_recovery_result_volume
cdm_disaster_recovery_result_tenant ||-r-o{ cdm_disaster_recovery_result_instance
cdm_disaster_recovery_result_network ||--o{ cdm_disaster_recovery_result_subnet
cdm_disaster_recovery_result_subnet ||-l-o{ cdm_disaster_recovery_result_subnet_dhcp_pool
cdm_disaster_recovery_result_subnet ||-l-o{ cdm_disaster_recovery_result_subnet_nameserver
cdm_disaster_recovery_result_subnet ||-r-o{ cdm_disaster_recovery_result_instance_network
cdm_disaster_recovery_result_router ||--o{ cdm_disaster_recovery_result_internal_routing_interface
cdm_disaster_recovery_result_router ||--o{ cdm_disaster_recovery_result_external_routing_interface
cdm_disaster_recovery_result_router ||-l-o{ cdm_disaster_recovery_result_extra_route
cdm_disaster_recovery_result_floating_ip |o--o{ cdm_disaster_recovery_result_instance_network
cdm_disaster_recovery_result_security_group ||--o{ cdm_disaster_recovery_result_security_group_rule
cdm_disaster_recovery_result_security_group ||-l-o{ cdm_disaster_recovery_result_instance_security_group
cdm_disaster_recovery_result_instance_spec ||-r-o{ cdm_disaster_recovery_result_instance_extra_spec
cdm_disaster_recovery_result_instance_spec ||--o{ cdm_disaster_recovery_result_instance
cdm_disaster_recovery_result_keypair ||--o{ cdm_disaster_recovery_result_instance
cdm_disaster_recovery_result_instance ||-r-o{ cdm_disaster_recovery_result_instance_dependency
cdm_disaster_recovery_result_instance ||--o{ cdm_disaster_recovery_result_instance_network
cdm_disaster_recovery_result_instance ||--o{ cdm_disaster_recovery_result_instance_security_group
cdm_disaster_recovery_result_instance ||--o{ cdm_disaster_recovery_result_instance_volume
cdm_disaster_recovery_result_volume ||--o{ cdm_disaster_recovery_result_instance_volume

@enduml
```
