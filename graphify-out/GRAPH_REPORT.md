# Graph Report - backend  (2026-07-22)

## Corpus Check
- 59 files · ~23,031 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 686 nodes · 1205 edges · 53 communities (28 shown, 25 thin omitted)
- Extraction: 100% EXTRACTED · 0% INFERRED · 0% AMBIGUOUS · INFERRED: 6 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `abeae510`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

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
- @eslint/eslintrc
- @eslint/js
- eslint-plugin-prettier
- globals
- jest
- @nestjs/schematics
- @nestjs/testing
- prettier
- source-map-support
- supertest
- ts-jest
- ts-loader
- ts-node
- @types/express
- @types/express-session
- @types/jest
- @types/node
- @types/nodemailer
- @types/passport-google-oauth20
- @types/supertest
- typescript
- typescript-eslint

## God Nodes (most connected - your core abstractions)
1. `User` - 53 edges
2. `ClassesService` - 45 edges
3. `ClassesController` - 42 edges
4. `Roles()` - 29 edges
5. `UsersService` - 28 edges
6. `compilerOptions` - 22 edges
7. `Class` - 21 edges
8. `Batch` - 18 edges
9. `UsersController` - 17 edges
10. `Assignment` - 16 edges

## Surprising Connections (you probably didn't know these)
- `bootstrap()` --indirect_call--> `Program`  [INFERRED]
  src/seed-dummy.ts → src/classes/entities/program.entity.ts
- `bootstrap()` --indirect_call--> `User`  [INFERRED]
  src/main.ts → src/users/entities/user.entity.ts
- `bootstrap()` --indirect_call--> `User`  [INFERRED]
  src/seed-dummy.ts → src/users/entities/user.entity.ts
- `bootstrap()` --indirect_call--> `AppModule`  [INFERRED]
  src/main.ts → src/app.module.ts
- `bootstrap()` --indirect_call--> `AppModule`  [INFERRED]
  src/seed-dummy.ts → src/app.module.ts

## Import Cycles
- None detected.

## Communities (53 total, 25 thin omitted)

### Community 0 - "Rumpun backend_src_classes_entities_class_entity_class"
Cohesion: 0.06
Nodes (14): Put, Roles(), ClassesController, Body, Controller, Delete, Get, Param (+6 more)

### Community 1 - "Rumpun backend_src_classes_entities_competency_entity_competency"
Cohesion: 0.06
Nodes (44): IsDateString, PrimaryColumn, Query, AttendanceController, Body, Controller, Get, Param (+36 more)

### Community 2 - "Rumpun backend_src_users_entities_user_entity_user"
Cohesion: 0.06
Nodes (38): IsEmail, AppModule, Module, AuthModule, Module, AuthService, GoogleUserPayload, Injectable (+30 more)

### Community 3 - "Rumpun backend_src_users_dto_bulk_invite_dto_bulkinvitedto"
Cohesion: 0.06
Nodes (22): RolesGuard, Injectable, SessionSerializer, Injectable, Column, CreateDateColumn, Entity, PrimaryGeneratedColumn (+14 more)

### Community 4 - "Rumpun backend_src_app_controller_appcontroller"
Cohesion: 0.04
Nodes (44): active, Admin, Akun suspended, API, Authentication Concept, Authentication Implementation Plan (Core), Authentication Module, Callback (+36 more)

### Community 5 - "Rumpun backend_src_app_module_appmodule"
Cohesion: 0.06
Nodes (35): class-transformer, class-validator, express-session, @nestjs/common, @nestjs/config, @nestjs/core, @nestjs/mapped-types, @nestjs/passport (+27 more)

### Community 6 - "Rumpun backend_src_app_service_appservice"
Cohesion: 0.06
Nodes (34): author, description, jest, collectCoverageFrom, coverageDirectory, moduleFileExtensions, rootDir, testEnvironment (+26 more)

### Community 7 - "Rumpun backend_src_auth_auth_controller_authcontroller"
Cohesion: 0.06
Nodes (31): 1. Program AI Development, 2. Program Game Development, 3. Program Web Development and UI/UX Design, 4. Program Mobile Development and UI/UX Design, Bab 4: Spesifikasi Rinci Program & Aturan Hubungan, Kepemilikan Student (Student Ownership), Kepemilikan Student (Student Ownership), Kepemilikan Student (Student Ownership) (+23 more)

### Community 8 - "Rumpun backend_src_auth_auth_module_authmodule"
Cohesion: 0.07
Nodes (28): 🔍 Cara Bertanya & Kueri Grafik, 🔄 Cara Melanjutkan & Memperbarui Grafik, 🛠️ Langkah Instalasi (Satu Kali Saja), Metode 1: Lewat Chat Google Antigravity (Rekomendasi), Metode 2: Lewat Terminal CLI (Headless / Offline), 📋 Persyaratan Awal (Prerequisites), 🚀 Tutorial Graphify — Tim Backend (LMS V2), 📊 Visualisasi Interaktif (+20 more)

### Community 9 - "Rumpun backend_src_auth_auth_service_authservice"
Cohesion: 0.09
Nodes (22): compilerOptions, allowSyntheticDefaultImports, baseUrl, declaration, emitDecoratorMetadata, esModuleInterop, experimentalDecorators, forceConsistentCasingInFileNames (+14 more)

### Community 10 - "Rumpun backend_src_auth_auth_service_googleuserpayload"
Cohesion: 0.35
Nodes (3): ClassesModule, Module, LogbookStatus

### Community 11 - "Rumpun backend_src_auth_guards_google_auth_guard_googleauthguard"
Cohesion: 0.31
Nodes (7): Res, AuthController, Controller, Get, Post, Req, UseGuards

### Community 12 - "Rumpun backend_src_auth_guards_roles_guard_rolesguard"
Cohesion: 0.29
Nodes (5): AppController, Controller, Get, AppService, Injectable

### Community 13 - "Rumpun backend_src_auth_guards_session_auth_guard_sessionauthguard"
Cohesion: 0.20
Nodes (9): Compile and run the project, Deployment, Description, License, Project setup, Resources, Run tests, Stay in touch (+1 more)

### Community 14 - "Rumpun backend_src_auth_serializers_session_serializer_sessionserializer"
Cohesion: 0.22
Nodes (8): InjectRepository, Enrollment, Column, CreateDateColumn, Entity, JoinColumn, ManyToOne, PrimaryGeneratedColumn

### Community 15 - "Rumpun backend_src_auth_strategies_google_strategy_googlestrategy"
Cohesion: 0.22
Nodes (9): Assignment, Column, CreateDateColumn, Entity, JoinColumn, ManyToOne, OneToMany, PrimaryGeneratedColumn (+1 more)

### Community 16 - "Rumpun backend_src_classes_classes_controller_classescontroller"
Cohesion: 0.22
Nodes (9): Class, Column, CreateDateColumn, Entity, JoinColumn, ManyToOne, OneToMany, PrimaryGeneratedColumn (+1 more)

### Community 17 - "Rumpun backend_src_classes_classes_module_classesmodule"
Cohesion: 0.25
Nodes (7): 1. Informasi Umum, 2. Spesifikasi Teknis & Sistem Desain, 3. Panduan Gaya (Style Guide), 4. Manajemen Peran (User Roles), 5. Kebutuhan Halaman: Landing Page, 6. Kebutuhan Halaman: Login Page, Product Requirements Document (PRD): Infinite Learning LMS

### Community 18 - "Rumpun backend_src_classes_classes_service_classesservice"
Cohesion: 0.25
Nodes (7): dist, node_modules, **/*spec.ts, test, ./tsconfig.json, exclude, extends

### Community 19 - "Rumpun backend_src_users_dto_update_user_dto_updateuserdto"
Cohesion: 0.25
Nodes (8): Competency, Column, CreateDateColumn, Entity, JoinColumn, ManyToOne, PrimaryGeneratedColumn, UpdateDateColumn

### Community 20 - "Rumpun backend_src_users_mail_service_mailservice"
Cohesion: 0.25
Nodes (8): Logbook, Column, CreateDateColumn, Entity, JoinColumn, ManyToOne, PrimaryGeneratedColumn, UpdateDateColumn

### Community 21 - "Rumpun backend_src_users_users_controller_userscontroller"
Cohesion: 0.25
Nodes (8): Material, Column, CreateDateColumn, Entity, JoinColumn, ManyToOne, PrimaryGeneratedColumn, UpdateDateColumn

### Community 22 - "Rumpun backend_src_users_users_module_usersmodule"
Cohesion: 0.25
Nodes (8): Submission, Column, CreateDateColumn, Entity, JoinColumn, ManyToOne, PrimaryGeneratedColumn, UpdateDateColumn

### Community 23 - "Rumpun backend_src_users_users_service_usersservice"
Cohesion: 0.29
Nodes (7): eslint, @nestjs/cli, devDependencies, eslint, @nestjs/cli, tsconfig-paths, tsconfig-paths

### Community 24 - "Rumpun frontend_app_dashboard_components_admin_dashboard_userlistitem"
Cohesion: 0.33
Nodes (5): IsNumber, Min, CreateLogbookDto, IsNotEmpty, IsString

### Community 25 - "Rumpun frontend_app_dashboard_page_dashboardpage"
Cohesion: 0.33
Nodes (5): collection, compilerOptions, deleteOutDir, $schema, sourceRoot

### Community 26 - "Rumpun frontend_app_layout_rootlayout"
Cohesion: 0.33
Nodes (6): Program, Column, CreateDateColumn, Entity, PrimaryGeneratedColumn, UpdateDateColumn

## Knowledge Gaps
- **206 isolated node(s):** `$schema`, `collection`, `sourceRoot`, `deleteOutDir`, `name` (+201 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **25 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `User` connect `Rumpun backend_src_users_dto_bulk_invite_dto_bulkinvitedto` to `Rumpun backend_src_classes_entities_competency_entity_competency`, `Rumpun backend_src_users_entities_user_entity_user`, `Rumpun backend_src_auth_auth_service_googleuserpayload`, `Rumpun backend_src_auth_guards_google_auth_guard_googleauthguard`, `Rumpun backend_src_auth_serializers_session_serializer_sessionserializer`, `Rumpun backend_src_classes_classes_controller_classescontroller`, `Rumpun backend_src_users_dto_update_user_dto_updateuserdto`, `Rumpun backend_src_users_mail_service_mailservice`, `Rumpun backend_src_users_users_module_usersmodule`?**
  _High betweenness centrality (0.098) - this node is a cross-community bridge._
- **Why does `ClassesService` connect `Rumpun backend_src_classes_entities_class_entity_class` to `Rumpun backend_src_auth_auth_service_googleuserpayload`, `Rumpun backend_src_auth_serializers_session_serializer_sessionserializer`?**
  _High betweenness centrality (0.054) - this node is a cross-community bridge._
- **Why does `ClassesController` connect `Rumpun backend_src_classes_entities_class_entity_class` to `Rumpun backend_src_auth_auth_service_googleuserpayload`?**
  _High betweenness centrality (0.050) - this node is a cross-community bridge._
- **Are the 2 inferred relationships involving `User` (e.g. with `bootstrap()` and `bootstrap()`) actually correct?**
  _`User` has 2 INFERRED edges - model-reasoned connections that need verification._
- **What connects `$schema`, `collection`, `sourceRoot` to the rest of the system?**
  _206 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Rumpun backend_src_classes_entities_class_entity_class` be split into smaller, more focused modules?**
  _Cohesion score 0.060542309490416085 - nodes in this community are weakly interconnected._
- **Should `Rumpun backend_src_classes_entities_competency_entity_competency` be split into smaller, more focused modules?**
  _Cohesion score 0.0553116769095698 - nodes in this community are weakly interconnected._