# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.0] - 2026-11-21

### Added

- add current Pod and Node metrics (#27)
- phase1e2 add native Helm release management (#26)
- replace default app icon with KubeSift mark (#25)
- Phase 1E.1 native Helm release inspection (#24)
- Phase 1D.8 reviewed server-side apply and closure audit (#23)
- add reviewed resource creation and collapsible sidebar sections (#22)
- add Pod terminals and TCP forwarding (#21)
- add reviewed workload rollback (#20)
- add reviewed YAML editing (#19)
- add reviewed workload deletion (#18)
- add reviewed workload restart (#17)
- add reviewed Deployment and StatefulSet scaling (#15)
- add cluster resources and installed API discovery (#14)
- add read-only availability investigation (#13)
- add read-only access control investigation (#12)
- add read-only storage investigation (#11)
- phase1c3c add read-only namespace policy and configuration (#10)
- phase1c3c add read-only namespace policy and configuration
- Phase 1C.3b read-only networking investigation (#9)
- Phase 1C.3a remaining workloads (#8)
- phase1c2 add read-only ReplicaSet investigation
- phase1c2 refine workload detail views and scope handling
- phase1c2-task6 share workload logs across detail and Sessions
- phase1c2-task5 connect workload investigation views
- phase1c2-task4 bind product logs to Pod identity
- add UID-scoped workload events
- add workload details and owned Pod navigation
- add bounded workload inventory reads

### Fixed

- refine resource detail inspection (#16)
- reject full-shaped responses on Secret metadata paths
- phase1c2-task7 bound high-volume log rendering
- phase1c2-task7 keep one live log view
- phase1c2-task7 simplify investigation and fit launch window
- phase1c2-task7 refine launch window and inventory controls
- phase1c2-task7 resolve review findings and validate investigation
- name the macOS app KubeSift

### Documentation

- describe internal packages in Markdown READMEs (#28)
- phase1e3-task4 record reviewed merge checkpoint
- specify and plan Phase 1E.3 current metrics
- phase1e2-task6 record confirmed develop merge
- record application icon merge
- specify and plan Phase 1E.2 Helm management
- record merged Phase 1E.1 Helm inspection
- specify and plan Phase 1E.1 native Helm inspection
- record merged Phase 1D.8 and close Phase 1D
- reconcile phase1d8 plan with merged creation baseline
- record merged Phase 1D.7 resource creation
- include bindings path normalization in phase1d7
- plan resource creation and apply to complete phase1d
- record merged Phase 1D.6 interactive sessions
- specify and plan Phase 1D.6 interactive sessions
- record merged Phase 1D.5 rollback
- plan Phase 1D.5 workload rollback
- specify Phase 1D.5 workload rollback
- record reviewed YAML editing merge
- plan simple Phase 1D.4 YAML editing
- simplify YAML editing around Kubernetes permissions
- specify Phase 1D.4 reviewed YAML editing
- record reviewed workload deletion merge
- plan phase1d3 reviewed workload deletion
- record reviewed workload restart merge
- plan phase1d2 reviewed workload restart
- record resource detail refinement merge
- record reviewed scaling merge in status
- plan phase1d1 reviewed workload scaling
- record phase1c3g merge in status
- keep one planning sidebar and list every validation report
- plan phase1c3g cluster resources and API discovery
- record phase1c3f merge in status
- plan phase1c3f availability and revision investigation
- record phase1c3e merge in status
- include workload configuration grouping in phase1c3e
- plan phase1c3e read-only access control
- record phase1c3d merge in status
- plan phase1c3d read-only storage investigation
- record Phase 1C.3c merge in status
- rewrite the development toolchain guide
- apply the planning design to the toolchain page
- consolidate planning design and index on delivery branch
- streamline the planning index
- separate progress tracking from roadmap
- plan phase1c3c configuration and order remaining roadmap
- require one independent whole-PR review before merge
- record Phase 1C.3b merge in status
- plan phase1c3b read-only networking investigation
- record Phase 1C.3a merge in status
- plan phase1c3a remaining workload investigation
- record Phase 1C.2 merge in status documents
- plan workload refinement and ReplicaSets explorer
- record Phase 1C.2 native UAT results
- phase1c2-task7 link unmerged UAT pull request
- phase1c2-task7 record validation and user UI UAT
- plan Phase 1C.2 workload investigation
- require agents to preserve approved UI conventions
- specify Phase 1C.2 workload investigation

## [0.3.0] - 2026-11-12

### Added

- compact workspace controls and Pod presentation
- refine workspace colours and document verified controls
- simplify namespace selection and resize Pod details
- persist Pod detail split preferences

### Fixed

- use the OpenShift circular-arrow icon
- distinguish Light Pod phases and prune historical images

### Documentation

- complete workspace plan and portable handoff
- record workspace refinement validation and review
- specify neutral dark palette and workspace controls

## [0.2.0] - 2026-11-12

### Added

- implement Phase 1C.1 cluster connections and live entry (#3)
- implement Phase 1B UI foundation (#2)
- complete Phase 1A desktop integration validation (#1)

### Documentation

- record Phase 1C.1 merge and qualified UAT
- improve release-this workflow
- plan Phase 1C.1 connections and user UI acceptance
- specify Phase 1C.1 cluster entry and connections
- define cluster-first entry and external login workflow
- capture live cluster entry feedback and fixture scope
- record Phase 1B merge and Phase 1C planning checkpoint
- allow planning updates directly on develop
- plan approved Phase 1B UI foundation
- plan Phase 1A and align open-source engineering guidance
- record the engineering loop and git workflow
- specify Phase 1A validation and implementation handoff
- establish KubeSift product and architecture planning
