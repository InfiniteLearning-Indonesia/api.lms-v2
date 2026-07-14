# Graph Report - ..  (2026-07-14)

## Corpus Check
- 116 files · ~51,252 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 36 nodes · 10 edges · 30 communities (1 shown, 29 thin omitted)
- Extraction: 100% EXTRACTED · 0% INFERRED · 0% AMBIGUOUS
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- Rumpun backend_src_classes_entities_class_entity_class
- Rumpun backend_src_classes_entities_competency_entity_competency
- Rumpun backend_src_users_entities_user_entity_user
- Rumpun backend_src_users_dto_bulk_invite_dto_bulkinvitedto
- Rumpun backend_src_app_controller_appcontroller
- Rumpun backend_src_app_module_appmodule
- Rumpun backend_src_app_service_appservice
- Rumpun backend_src_auth_auth_controller_authcontroller
- Rumpun backend_src_auth_auth_module_authmodule
- Rumpun backend_src_auth_auth_service_authservice
- Rumpun backend_src_auth_auth_service_googleuserpayload
- Rumpun backend_src_auth_guards_google_auth_guard_googleauthguard
- Rumpun backend_src_auth_guards_roles_guard_rolesguard
- Rumpun backend_src_auth_guards_session_auth_guard_sessionauthguard
- Rumpun backend_src_auth_serializers_session_serializer_sessionserializer
- Rumpun backend_src_auth_strategies_google_strategy_googlestrategy
- Rumpun backend_src_classes_classes_controller_classescontroller
- Rumpun backend_src_classes_classes_module_classesmodule
- Rumpun backend_src_classes_classes_service_classesservice
- Rumpun backend_src_users_dto_update_user_dto_updateuserdto
- Rumpun backend_src_users_mail_service_mailservice
- Rumpun backend_src_users_users_controller_userscontroller
- Rumpun backend_src_users_users_module_usersmodule
- Rumpun backend_src_users_users_service_usersservice
- Rumpun frontend_app_dashboard_components_admin_dashboard_userlistitem
- Rumpun frontend_app_dashboard_page_dashboardpage
- Rumpun frontend_app_layout_rootlayout
- Rumpun frontend_components_theme_provider_themeprovider
- Rumpun frontend_components_theme_toggle_themetoggle
- Rumpun frontend_lib_utils_cn

## God Nodes (most connected - your core abstractions)
1. `Class` - 6 edges
2. `User` - 3 edges
3. `Competency` - 2 edges
4. `Enrollment` - 2 edges
5. `Program` - 2 edges
6. `Assignment` - 1 edges
7. `Batch` - 1 edges
8. `Material` - 1 edges
9. `BulkInviteDto` - 1 edges
10. `CreateUserDto` - 1 edges

## Surprising Connections (you probably didn't know these)
- `Class` --references--> `Program`  [EXTRACTED]
  backend/src/classes/entities/class.entity.ts → backend/src/classes/entities/program.entity.ts
- `Enrollment` --references--> `Class`  [EXTRACTED]
  backend/src/classes/entities/enrollment.entity.ts → backend/src/classes/entities/class.entity.ts
- `Assignment` --references--> `Class`  [EXTRACTED]
  backend/src/classes/entities/assignment.entity.ts → backend/src/classes/entities/class.entity.ts
- `Class` --references--> `Batch`  [EXTRACTED]
  backend/src/classes/entities/class.entity.ts → backend/src/classes/entities/batch.entity.ts
- `Class` --references--> `Material`  [EXTRACTED]
  backend/src/classes/entities/class.entity.ts → backend/src/classes/entities/material.entity.ts

## Import Cycles
- None detected.

## Communities (30 total, 29 thin omitted)

### Community 0 - "Rumpun backend_src_classes_entities_class_entity_class"
Cohesion: 0.50
Nodes (4): Assignment, Batch, Class, Material

## Knowledge Gaps
- **31 isolated node(s):** `AppController`, `AppModule`, `AppService`, `AuthController`, `AuthModule` (+26 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **29 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Class` connect `Rumpun backend_src_classes_entities_class_entity_class` to `Rumpun backend_src_classes_entities_competency_entity_competency`, `Rumpun backend_src_users_entities_user_entity_user`?**
  _High betweenness centrality (0.028) - this node is a cross-community bridge._
- **Why does `User` connect `Rumpun backend_src_users_entities_user_entity_user` to `Rumpun backend_src_classes_entities_class_entity_class`, `Rumpun backend_src_classes_entities_competency_entity_competency`?**
  _High betweenness centrality (0.005) - this node is a cross-community bridge._
- **Why does `Program` connect `Rumpun backend_src_classes_entities_competency_entity_competency` to `Rumpun backend_src_classes_entities_class_entity_class`?**
  _High betweenness centrality (0.003) - this node is a cross-community bridge._
- **What connects `AppController`, `AppModule`, `AppService` to the rest of the system?**
  _31 weakly-connected nodes found - possible documentation gaps or missing edges._