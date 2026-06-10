CREATE TABLE `blobs` (
	`sha256` text(64) PRIMARY KEY NOT NULL,
	`size` integer NOT NULL,
	`ref_count` integer DEFAULT 0 NOT NULL,
	`created_at` integer NOT NULL,
	CONSTRAINT "ref_count_non_negative" CHECK("blobs"."ref_count" >= 0)
);
--> statement-breakpoint
CREATE TABLE `files` (
	`id` text(21) PRIMARY KEY NOT NULL,
	`folder_id` text NOT NULL,
	`name` text NOT NULL,
	`mime_type` text NOT NULL,
	`sha256` text(64) NOT NULL,
	`created_at` integer NOT NULL,
	`updated_at` integer NOT NULL,
	`deleted_at` integer,
	FOREIGN KEY (`folder_id`) REFERENCES `folders`(`id`) ON UPDATE restrict ON DELETE cascade,
	FOREIGN KEY (`sha256`) REFERENCES `blobs`(`sha256`) ON UPDATE restrict ON DELETE restrict
);
--> statement-breakpoint
CREATE UNIQUE INDEX `idx_files_name_in_folder` ON `files` (`name`,`folder_id`) WHERE "files"."deleted_at" is null;--> statement-breakpoint
CREATE INDEX `idx_files_folder_active` ON `files` (`folder_id`) WHERE "files"."deleted_at" is null;--> statement-breakpoint
CREATE INDEX `idx_files_sha256` ON `files` (`sha256`);--> statement-breakpoint
CREATE INDEX `idx_files_deleted` ON `files` (`deleted_at`);--> statement-breakpoint
CREATE TABLE `folders` (
	`id` text(21) PRIMARY KEY NOT NULL,
	`parent_id` text,
	`name` text NOT NULL,
	`created_at` integer NOT NULL,
	`updated_at` integer NOT NULL,
	`deleted_at` integer,
	FOREIGN KEY (`parent_id`) REFERENCES `folders`(`id`) ON UPDATE restrict ON DELETE cascade
);
--> statement-breakpoint
CREATE UNIQUE INDEX `idx_folders_name_in_parent` ON `folders` (`name`,`parent_id`) WHERE "folders"."deleted_at" is null;--> statement-breakpoint
CREATE INDEX `idx_folders_parent_active` ON `folders` (`parent_id`) WHERE "folders"."deleted_at" is null;--> statement-breakpoint
CREATE TABLE `shares` (
	`token` text(21) PRIMARY KEY NOT NULL,
	`file_id` text NOT NULL,
	`expires_at` integer,
	`password_hash` text,
	`download_limit` integer,
	`download_count` integer DEFAULT 0 NOT NULL,
	`revoked_at` integer,
	`created_at` integer NOT NULL,
	FOREIGN KEY (`file_id`) REFERENCES `files`(`id`) ON UPDATE restrict ON DELETE cascade,
	CONSTRAINT "share_download_limit_check" CHECK("shares"."download_limit" IS NULL OR "shares"."download_count" <= "shares"."download_limit")
);
--> statement-breakpoint
CREATE INDEX `idx_shares_file` ON `shares` (`file_id`);