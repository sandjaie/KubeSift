/*
Purpose: Adapt the dedicated preferences service to product presentation.
Used by: App and SettingsPage.
Responsibilities: Load/save only validated appearance and pane dimensions.
Important: No localStorage, credentials, resources or conversations are persisted.
*/
import {PreferencesService} from '../../bindings/github.com/sandjaie/KubeSift/internal/desktop';
import type {Preferences} from '../../bindings/github.com/sandjaie/KubeSift/internal/preferences/models';
export type {Preferences};
export const defaultPreferences=():Preferences=>({version:1,appearance:'system',window:null,navigationWidth:220,aiWidth:360,dockHeight:220});
export async function loadPreferences():Promise<Preferences>{return PreferencesService.GetPreferences();}
export async function savePreferences(value:Preferences):Promise<void>{await PreferencesService.SetPreferences(value);}
