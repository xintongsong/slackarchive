package migrations

import (
	"github.com/go-pg/migrations"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		_, err := db.Exec(`
			CREATE AGGREGATE tsvector_agg(tsvector) (
			 STYPE = pg_catalog.tsvector,
			 SFUNC = pg_catalog.tsvector_concat,
			 INITCOND = ''
			);

			CREATE FUNCTION tsvector_concat(tsvector[])
			 RETURNS tsvector
			 LANGUAGE sql
			AS $function$
				SELECT tsvector_agg(tsv) FROM unnest($1) as tsv
			$function$;


			CREATE TABLE public.teams (
					id text NOT NULL,
					name text,
					domain text,
					token text,
					is_disabled boolean,
					is_hidden boolean,
					plan text,
					icon jsonb,
					CONSTRAINT teams_pkey PRIMARY KEY (id)
			);


			CREATE TABLE public.users (
					id text NOT NULL,
					name text,
					team_id text,
					deleted boolean,
					color text,
					profile jsonb,
					is_bot boolean,
					is_admin boolean,
					is_owner boolean,
					is_primary_owner boolean,
					is_restricted boolean,
					is_ultra_restricted boolean,
					has_files boolean,
					presence text,
					CONSTRAINT users_pkey PRIMARY KEY (id),
					CONSTRAINT users_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id)
			);

			CREATE TABLE public.channels (
					id text NOT NULL,
					name text NOT NULL,
					team_id text NOT NULL,
					is_channel boolean,
					creator_id text,
					is_archived boolean NOT NULL,
					is_general boolean NOT NULL,
					is_group boolean NOT NULL,
					members text[],
					topic jsonb,
					purpose jsonb,
					is_member boolean,
					last_read text,
					unread_count bigint,
					num_members bigint NOT NULL,
					unread_count_display bigint,
					CONSTRAINT channels_pkey PRIMARY KEY (id),
					CONSTRAINT channels_creator_id_fkey FOREIGN KEY (creator_id) REFERENCES public.users(id),
					CONSTRAINT channels_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id)
			);


			CREATE TABLE public.messages (
					channel_id text NOT NULL,
					user_id text NOT NULL,
					"timestamp" timestamp with time zone NOT NULL,
					thread_timestamp timestamp with time zone,
					msg jsonb,
					tsv tsvector,
					CONSTRAINT messages_pkey PRIMARY KEY (channel_id, user_id, "timestamp"),
					CONSTRAINT messages_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channels(id),
					CONSTRAINT messages_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id)
			);

			CREATE INDEX messages_idx_msg ON public.messages USING gin (msg);
			CREATE INDEX messages_idx_channel ON public.messages USING btree (channel_id);
			CREATE INDEX messages_idx_subtype ON public.messages USING btree (((msg ->> 'subtype'::text)));
			CREATE INDEX messages_idx_tsv ON public.messages USING gin (tsv);

			CREATE OR REPLACE FUNCTION messages_upsert_trigger() RETURNS trigger AS $$
			begin
				new.tsv :=
					setweight(to_tsvector(coalesce(new.msg->>'text','')), 'A') ||
					setweight(
							tsvector_concat(
									array(
											SELECT to_tsvector(coalesce(a->>'title','')) || 
																		to_tsvector(coalesce(a->>'title_link',''))
											FROM jsonb_array_elements(new.msg->'attachments') AS a
									)
							), 'B');
				return new;
			end
			$$ LANGUAGE plpgsql;

			CREATE TRIGGER tsvectorupdate BEFORE INSERT OR UPDATE
					ON messages FOR EACH ROW EXECUTE PROCEDURE messages_upsert_trigger();
	`)
		return err
	}, func(db migrations.DB) error {
		_, err := db.Exec(`
			DROP TABLE messages;
			DROP TABLE channels;
			DROP TABLE users;
			DROP TABLE teams;
			DROP FUNCTION tsvector_concat(tsvector[]);
			DROP AGGREGATE tsvector_agg(tsvector);
		`)
		return err
	})
}
