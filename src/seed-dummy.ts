import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module.js';
import { DataSource } from 'typeorm';
import { Program } from './classes/entities/program.entity.js';
import { User, UserRole, UserStatus } from './users/entities/user.entity.js';

async function bootstrap() {
  const app = await NestFactory.createApplicationContext(AppModule);
  const dataSource = app.get(DataSource);

  console.log('Starting dummy data seeding...');

  try {
    const programRepo = dataSource.getRepository(Program);
    const userRepo = dataSource.getRepository(User);

    console.log('Cleaning up existing data...');
    await dataSource.query('TRUNCATE TABLE enrollments, materials, assignments, classes, competencies, users, batches, programs CASCADE;');

    console.log('Seeding Official Programs...');
    await programRepo.save(programRepo.create({ name: 'Web Development and UI/UX Design', description: 'Belajar membuat website dari nol hingga mahir serta desain UI/UX' }));
    await programRepo.save(programRepo.create({ name: 'AI Development', description: 'Membangun kecerdasan buatan' }));
    await programRepo.save(programRepo.create({ name: 'Mobile Development and UI/UX Design', description: 'Membuat aplikasi Android dan iOS serta desain UI/UX' }));
    await programRepo.save(programRepo.create({ name: 'Game Development', description: 'Membuat game dengan Unity/Unreal' }));

    console.log('Seeding Admin Account Only...');
    await userRepo.save(userRepo.create({
      email: 'arifiansaputra43@gmail.com',
      name: 'Arifian Saputra',
      roles: [UserRole.ADMIN],
      status: UserStatus.ACTIVE,
      whatsapp: '081234567890',
    }));

    console.log('✅ Admin account and 4 Official Programs seeded successfully! Database is completely clean and ready for scratch testing.');
  } catch (error: any) {
    console.error(`❌ Seeding failed: ${error.message}`);
  } finally {
    await app.close();
  }
}

bootstrap();
