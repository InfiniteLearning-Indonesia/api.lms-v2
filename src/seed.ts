import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module.js';
import { UsersService } from './users/users.service.js';
import { UserRole } from './users/entities/user.entity.js';

async function bootstrap() {
  const app = await NestFactory.createApplicationContext(AppModule);
  const usersService = app.get(UsersService);

  const args = process.argv.slice(2);
  const email = args[0];
  const name = args[1] || 'Admin';

  if (!email) {
    console.error('Usage: npx ts-node src/seed.ts <email> [<name>]');
    await app.close();
    process.exit(1);
  }

  try {
    const user = await usersService.invite({
      email,
      name,
      role: UserRole.ADMIN,
    });
    console.log(`Successfully seeded Admin user: ${user.name} (${user.email})`);
  } catch (error: any) {
    console.error(`Seeding failed: ${error.message}`);
  } finally {
    await app.close();
  }
}

bootstrap();
