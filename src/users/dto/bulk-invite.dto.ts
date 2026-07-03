import { IsArray, IsString, IsOptional, ValidateNested, IsEnum, IsBoolean } from 'class-validator';
import { Type } from 'class-transformer';
import { CreateUserDto } from './create-user.dto.js';
import { UserRole } from '../entities/user.entity.js';

export class BulkInviteDto {
  // Option 1: Structured JSON array
  @IsOptional()
  @IsArray()
  @ValidateNested({ each: true })
  @Type(() => CreateUserDto)
  users?: CreateUserDto[];

  // Option 2: Raw string (e.g. comma-separated or newline-separated emails)
  @IsOptional()
  @IsString()
  rawEmails?: string;

  // Default role to assign if using Option 2
  @IsOptional()
  @IsEnum(UserRole)
  defaultRole?: UserRole = UserRole.STUDENT;

  @IsOptional()
  @IsBoolean()
  sendEmail?: boolean;
}
