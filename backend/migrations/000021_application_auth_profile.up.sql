alter table applications
    add column auth_profile text not null default 'legacy'
        check (auth_profile in ('legacy', 'proof_bound'));
