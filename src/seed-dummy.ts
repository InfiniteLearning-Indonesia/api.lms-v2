import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module.js';
import { DataSource } from 'typeorm';
import { Program } from './classes/entities/program.entity.js';
import { Batch } from './classes/entities/batch.entity.js';
import { User, UserRole, UserStatus } from './users/entities/user.entity.js';
import { Class } from './classes/entities/class.entity.js';
import { Material } from './classes/entities/material.entity.js';
import { Assignment } from './classes/entities/assignment.entity.js';
import { Enrollment } from './classes/entities/enrollment.entity.js';
import { Competency } from './classes/entities/competency.entity.js';

async function bootstrap() {
  const app = await NestFactory.createApplicationContext(AppModule);
  const dataSource = app.get(DataSource);

  console.log('Starting dummy data seeding...');

  try {
    const programRepo = dataSource.getRepository(Program);
    const batchRepo = dataSource.getRepository(Batch);
    const userRepo = dataSource.getRepository(User);
    const classRepo = dataSource.getRepository(Class);
    const materialRepo = dataSource.getRepository(Material);
    const assignmentRepo = dataSource.getRepository(Assignment);
    const enrollmentRepo = dataSource.getRepository(Enrollment);
    const competencyRepo = dataSource.getRepository(Competency);

    console.log('Cleaning up existing data...');
    await dataSource.query('TRUNCATE TABLE enrollments, materials, assignments, classes, competencies, users, batches, programs CASCADE;');

    console.log('Seeding Programs...');
    const program1 = await programRepo.save(programRepo.create({ name: 'Web Development and UI/UX Design', description: 'Belajar membuat website dari nol hingga mahir serta desain UI/UX' }));
    const program2 = await programRepo.save(programRepo.create({ name: 'AI Development', description: 'Membangun kecerdasan buatan' }));
    const program3 = await programRepo.save(programRepo.create({ name: 'Mobile Development and UI/UX Design', description: 'Membuat aplikasi Android dan iOS serta desain UI/UX' }));
    const program4 = await programRepo.save(programRepo.create({ name: 'Game Development', description: 'Membuat game dengan Unity/Unreal' }));

    console.log('Seeding Batches...');
    const batch7 = await batchRepo.save(batchRepo.create({ name: 'Batch 7 - 2026', status: 'active' }));

    console.log('Seeding Users (Mentors & Students)...');
    const mentor1 = await userRepo.save(userRepo.create({
      email: 'febriyann.personal@gmail.com',
      name: 'Riyanda Azis Febrian',
      role: UserRole.MENTOR,
      status: UserStatus.ACTIVE,
      whatsapp: '081263666474',
      selectedProgram: 'Web Development and UI/UX Design',
      specialization: 'Mentor Web',
    }));
    const mentor2 = await userRepo.save(userRepo.create({
      email: 'mentor.ai@example.com',
      name: 'Budi AI',
      role: UserRole.MENTOR,
      status: UserStatus.ACTIVE,
      whatsapp: '081234567891',
      selectedProgram: 'AI Development',
      specialization: 'Mentor AI',
    }));

    console.log('Seeding Competencies...');
    await competencyRepo.save(competencyRepo.create({
      programId: program1.id,
      creatorMentorId: mentor1.id,
      name: 'Kompetensi 1: Fundamental Web Development',
      description: 'Dasar HTML5, CSS Modern (Flexbox/Grid), dan JavaScript ES6+',
      category: 'Technical',
    }));
    await competencyRepo.save(competencyRepo.create({
      programId: program1.id,
      creatorMentorId: mentor1.id,
      name: 'Kompetensi 2: Soft Skills (CCA)',
      description: 'Communication, Critical Thinking, dan Leadership',
      category: 'Soft Skills (CCA)',
    }));


    const student = await userRepo.save(userRepo.create({
      email: 'riyandaazis00@gmail.com',
      name: 'Student Riyanda',
      role: UserRole.STUDENT,
      status: UserStatus.ACTIVE,
      whatsapp: '081263666474',
      selectedProgram: 'Web Development and UI/UX Design',
    }));

    console.log('Seeding Classes...');
    const classWeb = await classRepo.save(classRepo.create({
      id: '550e8400-e29b-41d4-a716-446655440000', // Hardcoded for frontend mock integration
      programId: program1.id,
      batchId: batch7.id,
      mentorId: mentor1.id,
    }));

    const classAi = await classRepo.save(classRepo.create({
      id: '550e8400-e29b-41d4-a716-446655440001',
      programId: program2.id,
      batchId: batch7.id,
      mentorId: mentor2.id,
    }));

    console.log('Seeding Enrollments...');
    await enrollmentRepo.save(enrollmentRepo.create({
      studentId: student.id,
      classId: classWeb.id,
    }));

    console.log('Seeding Materials & Assignments...');
    await materialRepo.save(materialRepo.create({
      classId: classWeb.id,
      title: 'Pengenalan Web Semantik & HTML5',
      type: 'pdf',
      url: '#',
      content: 'Materi ini membahas tentang fundamental HTML5',
      competency: 'Kompetensi 1: Fundamental Web',
    }));
    await materialRepo.save(materialRepo.create({
      classId: classWeb.id,
      title: 'Arsitektur CSS Modern (Flexbox & Grid)',
      type: 'video',
      url: '#',
      content: 'Video pembelajaran layout modern dengan CSS',
      competency: 'Kompetensi 1: Fundamental Web',
    }));

    await materialRepo.save(materialRepo.create({
      classId: classWeb.id,
      title: 'Pengenalan JavaScript Modern (ES6+)',
      type: 'video',
      url: '#',
      content: 'Dasar-dasar interaktivitas dengan JS ES6+',
      competency: 'Kompetensi 2: Interaktivitas Web',
    }));

    const futureDate = new Date();
    futureDate.setDate(futureDate.getDate() + 7);
    await assignmentRepo.save(assignmentRepo.create({
      classId: classWeb.id,
      title: 'Tugas Praktik 1: Membuat Layout Responsive',
      description: 'Buat layout menggunakan Flexbox berdasarkan desain figma berikut.',
      dueDate: futureDate,
      competency: 'Kompetensi 1: Fundamental Web',
    }));

    await assignmentRepo.save(assignmentRepo.create({
      classId: classWeb.id,
      title: 'Tugas Praktik 2: DOM Manipulation',
      description: 'Buatlah To-Do List app sederhana dengan JavaScript murni.',
      dueDate: futureDate,
      competency: 'Kompetensi 2: Interaktivitas Web',
    }));

    console.log('✅ Dummy data seeded successfully!');
  } catch (error: any) {
    console.error(`❌ Seeding failed: ${error.message}`);
  } finally {
    await app.close();
  }
}

bootstrap();
