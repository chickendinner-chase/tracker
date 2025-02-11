import TelegramBot, { InlineKeyboardMarkup } from 'node-telegram-bot-api'
import { AddCommand } from '../commands/add-command'
import { START_MENU, SUB_MENU } from '../../config/bot-menus'
import { ManageCommand } from '../commands/manage-command'
import { DeleteCommand } from '../commands/delete-command'
import { userExpectingDonation, userExpectingGroupId, userExpectingWalletAddress, userExpectingCustomValue } from '../../constants/flags'
import { MyWalletCommand } from '../commands/mywallet-command'
import { GeneralMessages } from '../messages/general-messages'
import { UpgradePlanCommand } from '../commands/upgrade-plan-command'
import { UpgradePlanHandler } from './upgrade-plan-handler'
import { DonateCommand } from '../commands/donate-command'
import { DonateHandler } from './donate-handler'
import { SettingsCommand } from '../commands/settings-command'
import { UpdateBotStatusHandler } from './update-bot-status-handler'
import { PromotionHandler } from './promotion-handler'
import { GET_50_WALLETS_PROMOTION } from '../../constants/promotions'
import { PrismaUserRepository } from '../../repositories/prisma/user'
import { GroupsCommand } from '../commands/groups-command'
import { HelpCommand } from '../commands/help-command'

export class CallbackQueryHandler {
  private addCommand: AddCommand
  private manageCommand: ManageCommand
  private deleteCommand: DeleteCommand
  private myWalletCommand: MyWalletCommand
  private upgradePlanCommand: UpgradePlanCommand
  private donateCommand: DonateCommand
  private settingsCommand: SettingsCommand
  private groupsCommand: GroupsCommand
  private helpCommand: HelpCommand

  private updateBotStatusHandler: UpdateBotStatusHandler

  private prismaUserRepository: PrismaUserRepository

  private upgradePlanHandler: UpgradePlanHandler
  private donateHandler: DonateHandler
  private promotionHandler: PromotionHandler
  constructor(private bot: TelegramBot) {
    this.bot = bot

    this.addCommand = new AddCommand(this.bot)
    this.manageCommand = new ManageCommand(this.bot)
    this.deleteCommand = new DeleteCommand(this.bot)
    this.myWalletCommand = new MyWalletCommand(this.bot)
    this.upgradePlanCommand = new UpgradePlanCommand(this.bot)
    this.donateCommand = new DonateCommand(this.bot)
    this.settingsCommand = new SettingsCommand(this.bot)
    this.groupsCommand = new GroupsCommand(this.bot)
    this.helpCommand = new HelpCommand(this.bot)

    this.updateBotStatusHandler = new UpdateBotStatusHandler(this.bot)

    this.prismaUserRepository = new PrismaUserRepository()

    this.upgradePlanHandler = new UpgradePlanHandler(this.bot)
    this.donateHandler = new DonateHandler(this.bot)
    this.promotionHandler = new PromotionHandler(this.bot)
  }

  public call() {
    this.bot.on('callback_query', async (callbackQuery) => {
      const message = callbackQuery.message
      const chatId = message?.chat.id
      const data = callbackQuery.data

      const userId = message?.chat.id.toString()

      if (!chatId || !userId) {
        return
      }

      let responseText

      // handle donations
      if (data?.startsWith('donate_action')) {
        const donationAmount = data.split('_')[2]
        console.log(`User wants to donate ${donationAmount} SOL`)
        await this.donateHandler.makeDonation(message, Number(donationAmount))
        return
      }

      switch (data) {
        case 'add':
          this.addCommand.addButtonHandler(message)
          break
        case 'manage':
          await this.manageCommand.manageButtonHandler(message)
          break
        case 'delete':
          this.deleteCommand.deleteButtonHandler(message)
          break
        case 'settings':
          this.settingsCommand.settingsCommandHandler(message)
          break
        case 'pause-resume-bot':
          await this.updateBotStatusHandler.pauseResumeBot(message)
          break
        case 'upgrade':
          this.upgradePlanCommand.upgradePlanButtonHandler(message)
          break
        case 'upgrade_hobby':
          await this.upgradePlanHandler.upgradePlan(message, 'HOBBY')
          break
        case 'upgrade_pro':
          await this.upgradePlanHandler.upgradePlan(message, 'PRO')
          break
        case 'upgrade_whale':
          await this.upgradePlanHandler.upgradePlan(message, 'WHALE')
          break
        case 'donate':
          await this.donateCommand.donateCommandHandler(message)
          break
        case 'groups':
          await this.groupsCommand.groupsButtonHandler(message)
          break
        case 'delete_group':
          await this.groupsCommand.deleteGroupButtonHandler(message)
          break
        case 'help':
          this.helpCommand.helpButtonHandler(message)
          break
        case 'my_wallet':
          this.myWalletCommand.myWalletCommandHandler(message)
          break
        case 'show_private_key':
          this.myWalletCommand.showPrivateKeyHandler(message)
          break
        case 'buy_promotion':
          this.promotionHandler.buyPromotion(message, GET_50_WALLETS_PROMOTION.price, GET_50_WALLETS_PROMOTION.type)
          break
        case 'back_to_main_menu':
          const user = await this.prismaUserRepository.getById(userId)
          const messageText = GeneralMessages.startMessage(user)

          // reset any flags
          userExpectingWalletAddress[chatId] = false
          userExpectingDonation[chatId] = false
          userExpectingGroupId[chatId] = false

          this.bot.editMessageText(messageText, {
            chat_id: chatId,
            message_id: message.message_id,
            reply_markup: START_MENU,
            parse_mode: 'HTML',
          })
          break
        case 'filter_all':
          await this.updateWalletFilter(message, 'all')
          break
        case 'filter_buys':
          await this.updateWalletFilter(message, 'buys')
          break
        case 'filter_sells':
          await this.updateWalletFilter(message, 'sells')
          break
        case 'filter_high_value':
          const HIGH_VALUE_MENU: InlineKeyboardMarkup = {
            inline_keyboard: [
              [
                { text: '> 1 SOL', callback_data: 'high_value_1' },
                { text: '> 5 SOL', callback_data: 'high_value_5' }
              ],
              [
                { text: '> 10 SOL', callback_data: 'high_value_10' },
                { text: 'Custom', callback_data: 'high_value_custom' }
              ],
              [{ text: 'Back', callback_data: 'back_to_filters' }]
            ]
          }
          
          this.bot.editMessageText(
            '🔥 Select minimum transaction value to track:',
            {
              chat_id: message.chat.id,
              message_id: message.message_id,
              reply_markup: HIGH_VALUE_MENU
            }
          )
          break
        case 'high_value_1':
          await this.updateWalletFilter(message, 'high_value:1')
          break
        case 'high_value_5':
          await this.updateWalletFilter(message, 'high_value:5')
          break
        case 'high_value_10':
          await this.updateWalletFilter(message, 'high_value:10')
          break
        case 'high_value_custom':
          // Ask user for custom value
          this.bot.editMessageText(
            '💰 Enter minimum transaction value in SOL:',
            {
              chat_id: message.chat.id,
              message_id: message.message_id
            }
          )
          // Set flag to expect custom value
          userExpectingCustomValue[message.chat.id] = true
          break
        default:
          responseText = 'Unknown command.'
      }

      // this.bot.sendMessage(chatId, responseText);
    })
  }

  private async updateWalletFilter(message: TelegramBot.Message, filter: string) {
    const chatId = message.chat.id
    const userId = chatId.toString()
    
    await this.prismaUserRepository.updateFilter(userId, filter)
    
    let filterText = ''
    if (filter.startsWith('high_value:')) {
      const value = filter.split(':')[1]
      filterText = `transactions above ${value} SOL`
    } else {
      filterText = `${filter} transactions`
    }
    
    this.bot.editMessageText(
      `✅ Filter updated! You will now receive notifications for ${filterText}.`,
      {
        chat_id: chatId,
        message_id: message.message_id,
        reply_markup: SUB_MENU
      }
    )
  }

  private async handleCustomValue(message: TelegramBot.Message) {
    const text = message.text
    if (!text) return

    const value = parseFloat(text)
    if (isNaN(value) || value <= 0) {
      this.bot.sendMessage(
        message.chat.id,
        '❌ Please enter a valid number greater than 0'
      )
      return
    }

    await this.updateWalletFilter(message, `high_value:${value}`)
    userExpectingCustomValue[message.chat.id] = false
  }
}
