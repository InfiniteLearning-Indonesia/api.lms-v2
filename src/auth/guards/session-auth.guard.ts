import {
  CanActivate,
  ExecutionContext,
  Injectable,
  UnauthorizedException,
} from '@nestjs/common';
import { UsersService } from '../../users/users.service.js';

@Injectable()
export class SessionAuthGuard implements CanActivate {
  constructor(private readonly usersService: UsersService) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const request = context.switchToHttp().getRequest();
    if (!request.session?.userId) {
      throw new UnauthorizedException('Please log in to access this resource.');
    }

    const user = await this.usersService.findById(request.session.userId);
    if (!user) {
      throw new UnauthorizedException('User session is invalid or user not found.');
    }

    request.user = user;
    return true;
  }
}
