import { IsString, IsNotEmpty, IsArray, IsOptional } from 'class-validator';

export class CreatePermissionRequestDto {
  @IsString()
  @IsNotEmpty()
  studentId: string;

  @IsString()
  @IsNotEmpty()
  batchId: string;

  @IsString()
  @IsNotEmpty()
  date: string; // YYYY-MM-DD

  @IsString()
  @IsNotEmpty()
  category: string; // 'Izin' or 'Sakit'

  @IsString()
  @IsNotEmpty()
  reason: string;

  @IsArray()
  @IsOptional()
  proofFiles?: string[];

  @IsArray()
  @IsOptional()
  mentorChatFiles?: string[];
}
